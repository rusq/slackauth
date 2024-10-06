package slackauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/proto"
)

const domain = ".slack.com"

type Option func(*options)

type options struct {
	cookies     []*http.Cookie
	userAgent   string
	autoTimeout time.Duration

	// codeFn is the function that is called when slack does not recognise the
	// browser and challenges the user with a code sent to email.  it must
	// return the user-entered code.
	codeFn func(email string) (code int, err error)
	debug  bool
	lg     Logger
}

func (o *options) apply(opts []Option) {
	for _, opt := range opts {
		opt(o)
	}
}

// WithNoConsentPrompt adds a cookie that disables the Cookie Consent banner.
func WithNoConsentPrompt() Option {
	return func(o *options) {
		cookie := &http.Cookie{
			Domain:  domain,
			Path:    "/",
			Name:    "OptanonAlertBoxClosed",
			Value:   time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			Expires: time.Now().AddDate(0, 0, 30),
		}
		WithCookie(cookie)(o)
	}
}

// WithCookie adds a cookie to the request.
func WithCookie(cookie ...*http.Cookie) Option {
	return func(o *options) {
		o.cookies = append(o.cookies, cookie...)
	}
}

// WithUserAgent sets the user agent for the session.
func WithUserAgent(ua string) Option {
	return func(o *options) {
		if ua != "" {
			o.userAgent = ua
		}
	}
}

// Client is a Slackauth client.
type Client struct {
	wspURL    string
	cleanupFn []func() error
	opts      options
}

// New creates a new Slackauth client.
func New(workspace string, opt ...Option) (*Client, error) {
	wspURL, err := workspaceURL(workspace)
	if err != nil {
		return nil, err
	}
	if err := checkWorkspaceURL(wspURL); err != nil {
		return nil, err
	}

	opts := options{
		lg:          slog.Default(),
		codeFn:      SimpleChallengeFn,
		autoTimeout: 40 * time.Second, // default auto-login timeout
	}
	opts.apply(opt)

	return &Client{
		wspURL: wspURL,
		opts:   opts,
	}, nil
}

func (c *Client) Close() error {
	var errs error
	slices.Reverse(c.cleanupFn)
	for _, fn := range c.cleanupFn {
		if err := fn(); err != nil {
			errs = errors.Join(err, err)
		}
	}
	return errs
}

func WithLogger(l Logger) Option {
	return func(o *options) {
		o.lg = l
	}
}

// WithChallengeFunc sets the function that is called when slack does not
// recognise the browser and challenges the user with a code sent to email.
// All the function has to do is to accept the user input and return the code.
//
// See [SimpleChallengeFn](#SimpleChallengeFn) for an example.
func WithChallengeFunc(fn func(email string) (code int, err error)) Option {
	return func(o *options) {
		o.codeFn = fn
	}
}

// WithDebug enables debug logging.
func WithDebug(b bool) Option {
	return func(o *options) {
		o.debug = b
	}
}

func WithAutologinTimeout(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.autoTimeout = d
		}
	}
}

var (
	// ErrInvalidCredentials indicates that the credentials were invalid.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrLoginError indicates that some error of unknown nature occurred
	// during login.
	ErrLoginError = errors.New("slack reported an error during login")
	// ErrWorkspaceNotFound indicates that the workspace name was invalid.
	ErrWorkspaceNotFound = errors.New("workspace not found")
)

// ErrBadWorkspace is returned when the workspace name is invalid.
type ErrBadWorkspace struct {
	Name string
}

func (e ErrBadWorkspace) Error() string {
	return fmt.Sprintf("invalid workspace name: %q", e.Name)
}

// ErrBrowser indicates the error with browser interaction.
type ErrBrowser struct {
	Err      error
	FailedTo string
}

func (e ErrBrowser) Error() string {
	return fmt.Sprintf("browser automation error: failed to %s: %v", e.FailedTo, e.Err)
}

// Logger is the interface for the logger.
type Logger interface {
	// Debug logs a debug message.
	Debug(msg string, keyvals ...interface{})
}

func isURLSafe(s string) bool {
	for _, r := range s {
		if !isURLSafeRune(r) {
			return false
		}
	}
	return true
}

func isURLSafeRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '.' || r == '~'
}

func workspaceURL(workspace string) (string, error) {
	if !isURLSafe(workspace) {
		return "", ErrBadWorkspace{Name: workspace}
	}
	return "https://" + workspace + domain + "/", nil
}

// withTabGuard creates a context that is cancelled when the target is
// destroyed.
func withTabGuard(parent context.Context, browser *rod.Browser, targetID proto.TargetTargetID, l Logger) (context.Context, context.CancelCauseFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	go browser.EachEvent(func(e *proto.TargetTargetDestroyed) {
		if e.TargetID != targetID {
			// skipping unrelated target (user opened pages)
			return
		}
		l.Debug("target destroyed", "target", e.TargetID)
		cancel(errors.New("target page is closed"))
	})()
	return ctx, cancel
}

// checkWorkspaceURL checks if the workspace exists.  Slack returns 200 on
// existing workspaces and 404 on non-existing ones.
func checkWorkspaceURL(uri string) error {
	// quick status check
	if resp, err := http.Head(uri); err != nil {
		return ErrWorkspaceNotFound
	} else if resp.StatusCode != http.StatusOK {
		return ErrWorkspaceNotFound
	}
	return nil
}

// convertCookies extracts cookies from the browser and returns them as a
// slice of http.Cookie.
func convertCookies(cook []*proto.NetworkCookie, err error) ([]*http.Cookie, error) {
	if err != nil {
		return nil, fmt.Errorf("browser error: %w", err)
	}
	var cookies = make([]*http.Cookie, 0, len(cook))
	for _, c := range cook {
		sameSite, ok := sameSiteMap[c.SameSite]
		if !ok {
			sameSite = http.SameSiteNoneMode
		}
		cookies = append(cookies, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  c.Expires.Time(),
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
			SameSite: sameSite,
		})
	}
	return cookies, nil
}

var sameSiteMap = map[proto.NetworkCookieSameSite]http.SameSite{
	proto.NetworkCookieSameSiteNone:   http.SameSiteNoneMode,
	proto.NetworkCookieSameSiteLax:    http.SameSiteLaxMode,
	proto.NetworkCookieSameSiteStrict: http.SameSiteStrictMode,
}

func (c *Client) atClose(fn func() error) {
	c.cleanupFn = append(c.cleanupFn, fn)
}

func (c *Client) startBrowser(ctx context.Context) (*rod.Browser, error) {
	l := browserLauncher(false)
	url, err := l.Context(ctx).Launch()
	if err != nil {
		return nil, ErrBrowser{Err: err, FailedTo: "launch"}
	}
	c.atClose(toerrfn(l.Cleanup))

	browser := rod.New().Context(ctx).ControlURL(url)
	if err := browser.Connect(); err != nil {
		return nil, ErrBrowser{Err: err, FailedTo: "connect"}
	}
	browser.DefaultDevice(devices.Clear)
	c.atClose(browser.Close)
	return browser, nil
}

func (c *Client) navigate(b *rod.Browser) (*rod.Page, error) {
	if err := setCookies(b, c.opts.cookies); err != nil {
		return nil, err
	}

	page, err := b.Page(proto.TargetCreateTarget{URL: c.wspURL})
	if err != nil {
		return nil, ErrBrowser{Err: err, FailedTo: "open page"}
	}

	// patch the user agent if needed
	if err := c.opts.setUserAgent(page); err != nil {
		return nil, ErrBrowser{Err: err, FailedTo: "set user agent"}
	}

	return page, nil
}

func toerrfn(fn func()) func() error {
	return func() error {
		fn()
		return nil
	}
}
