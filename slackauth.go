package slackauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/trace"
	"slices"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const domain = ".slack.com"

type Option func(*options)

type options struct {
	cookies     []*http.Cookie
	userAgent   string
	autoTimeout time.Duration
	forceUser   bool // forces opening a browser with user data, instead of the clean one

	useBundledBrwsr bool   // forces using a bundled browser
	localBrowser    string // path to the local browser binary

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

// Client is a Slackauth client.  Zero value is not usable, use [New] to
// create a new client.
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

// Close closes the client and cleans up resources.
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

// WithForceUser forces the client to try to use the user's browser.  Using
// the user's browser can be used to avoid bot-detection mechanisms, as per
// this [rod issue].
//
// [rod issue]: https://github.com/go-rod/rod/issues/1033
func WithForceUser() Option {
	return func(o *options) {
		o.forceUser = true
	}
}

// WithBundledBrowser forces the client to use the bundled browser.
func WithBundledBrowser() Option {
	return func(o *options) {
		o.useBundledBrwsr = true
	}
}

func WithLocalBrowser(path string) Option {
	return func(o *options) {
		o.localBrowser = path
	}
}

// WithLogger sets the logger for the client.
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

// WithAutoLoginTimeout sets the timeout for the auto-login method.  The
// default is 40 seconds.  This is the net time needed for the automation
// process to complete, it does not include the time needed to start the
// browser, or navigate to the login page.
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
	// ErrInvalidChallengeCode indicates that the challenge code was invalid.
	ErrInvalidChallengeCode = errors.New("invalid challenge code")
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
	go browser.EachEvent(func(e *proto.TargetTargetDestroyed) bool {
		if e.TargetID != targetID {
			// skipping unrelated target (user opened pages)
			return false
		}
		l.Debug("target destroyed", "target", e.TargetID)
		cancel(errors.New("target page is closed"))
		return true
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
	ctx, task := trace.NewTask(ctx, "startBrowser")
	defer task.End()

	var l *launcher.Launcher
	if c.opts.forceUser {
		l = c.usrBrwsrLauncher()
	} else {
		l = c.newBrwsrLauncher(false)
	}
	url, err := l.Context(ctx).Launch()
	if err != nil {
		return nil, ErrBrowser{Err: err, FailedTo: "launch, you may need to close your browser first"}
	}
	c.atClose(toerrfn(l.Cleanup))

	browser := rod.New().Context(ctx).ControlURL(url).DefaultDevice(devices.Clear)
	if err := browser.Connect(); err != nil {
		return nil, ErrBrowser{Err: err, FailedTo: "connect"}
	}
	c.atClose(browser.Close)
	return browser, nil
}

// openSlackAuthTab opens the Slack workspace login screen in a new tab and
// initialises request hijacking.
func (c *Client) openSlackAuthTab(ctx context.Context, b *rod.Browser) (*rod.Page, *hijacker, error) {
	ctx, task := trace.NewTask(ctx, "openSlackAuthTab")
	defer task.End()

	if err := setCookies(b, c.opts.cookies); err != nil {
		return nil, nil, err
	}

	// we open the empty page first to be able to setup everything that we
	// desire before hitting slack workspace login page.
	pg, err := b.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, nil, ErrBrowser{Err: err, FailedTo: "create blank page"}
	}
	wait := pg.MustWaitNavigation()

	// set up the request hijacker
	h, err := newHijacker(ctx, pg, c.opts.lg)
	if err != nil {
		return nil, nil, ErrBrowser{Err: err, FailedTo: "create hijacker"}
	}
	c.atClose(h.Stop)
	// patch the user agent if needed
	if err := c.opts.setUserAgent(pg); err != nil {
		return nil, nil, ErrBrowser{Err: err, FailedTo: "set user agent"}
	}
	wait()

	// now we're ready, navigating to the slack workspace.  If we're running
	// in the user browser, the traps for the requests are already in place,
	// so the moment web-client opens, we might already get everything we
	// need.  If not, the user must be instructed to hit F5 or Cmd+R to
	// refresh.
	if err := pg.Navigate(c.wspURL + pathPwdSignin); err != nil {
		return nil, nil, ErrBrowser{Err: err, FailedTo: "navigate to login page"}
	}
	if err := pg.WaitLoad(); err != nil {
		return nil, nil, ErrBrowser{Err: err, FailedTo: "load page"}
	}
	c.atClose(pg.Close)

	return pg, h, nil
}

func toerrfn(fn func()) func() error {
	return func() error {
		fn()
		return nil
	}
}

func filterCookies(cookies []*http.Cookie) []*http.Cookie {
	// accepted contains the domain substrings that we want to keep cookies for.
	var accepted = []string{
		"slack.com",
		"google",
		"apple.com",
		"auth0.com",
		"okta.com",
		"onelogin.com",
	}

	var out = make([]*http.Cookie, 0, len(cookies))
LOOP:
	for _, c := range cookies {
		if c.Domain == "" {
			continue
		}
		for _, a := range accepted {
			if strings.Contains(c.Domain, a) {
				out = append(out, c)
				continue LOOP
			}
		}
	}
	return out
}

// trapRedirect traps the redirect page, and clicks the redirect when it
// appears.
func (c *Client) trapRedirect(ctx context.Context, page *rod.Page) (context.Context, func(cause error)) {
	ctxT, task := trace.NewTask(ctx, "trapRedirect")
	defer task.End()

	ctxTC, cancel := context.WithCancelCause(ctxT)

	trappedPg := page.Context(ctxTC)
	rctx := trappedPg.Race().Element(idRedirect).Handle(click)
	// sets the trap, which uses trappedPg context
	go func() {
		_, task := trace.NewTask(ctxTC, "race_do")
		defer task.End()

		rctx.Do()
	}()

	return ctx, cancel
}

// clicks the element el once with left mouse button.
func click(el *rod.Element) error {
	rgn := trace.StartRegion(el.Page().GetContext(), "click")
	defer rgn.End()
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return ErrBrowser{Err: err, FailedTo: "click the redirect link"}
	}
	return nil
}
