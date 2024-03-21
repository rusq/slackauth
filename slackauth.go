package slackauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const domain = ".slack.com"

type Option func(*options)

type options struct {
	cookies []*http.Cookie

	// codeFn is the function that is called when slack does not recognise the browser and challenges the user with a code sent to email.
	// it must return the user-entered code.
	codeFn func(email string) (code int, err error)
	debug  bool
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
func withTabGuard(parent context.Context, browser *rod.Browser, targetID proto.TargetTargetID) (context.Context, context.CancelCauseFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	go browser.EachEvent(func(e *proto.TargetTargetDestroyed) {
		if e.TargetID != targetID {
			// skipping unrelated target (user opened pages)
			return
		}
		slog.Debug("target destroyed", "target", e.TargetID)
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

// extractCookies extracts cookies from the browser and returns them as a
// slice of http.Cookie.
func extractCookies(browser *rod.Browser) ([]*http.Cookie, error) {
	cook, err := browser.GetCookies()
	if err != nil {
		return nil, err
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
