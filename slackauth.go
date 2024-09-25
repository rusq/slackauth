package slackauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const domain = ".slack.com"

type Option func(*options)

type options struct {
	cookies []*http.Cookie

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

type Logger interface {
	// Debug logs a debug message.
	Debug(msg string, keyvals ...interface{})
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

func browserLauncher(headless bool) *launcher.Launcher {
	var l *launcher.Launcher
	if binpath, ok := lookPath(); ok {
		l = launcher.New().
			Bin(binpath).
			Headless(headless).
			Leakless(isLeaklessEnabled). // Causes false positive on Windows, see #260
			Devtools(false)
	} else {
		l = launcher.New().
			Leakless(isLeaklessEnabled). // Causes false positive on Windows, see #260
			Headless(headless).
			Devtools(false)
	}
	return l
}

// lookPath is extended launcher.LookPath that includes support for Brave
// browser.
//
// (c) MIT license: Copyright 2019 Yad Smood
func lookPath() (found string, has bool) {
	list := map[string][]string{
		"darwin": {
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/google-chrome",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		},
		"linux": {
			"brave-browser",
			"chrome",
			"google-chrome",
			"/usr/bin/brave-browser",
			"/usr/bin/google-chrome",
			"microsoft-edge",
			"/usr/bin/microsoft-edge",
			"chromium",
			"chromium-browser",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/data/data/com.termux/files/usr/bin/chromium-browser",
		},
		"openbsd": {
			"chrome",
			"chromium",
		},
		"windows": append([]string{"chrome", "edge"}, expandWindowsExePaths(
			`BraveSoftware\Brave-Browser\Application\brave.exe`,
			`Google\Chrome\Application\chrome.exe`,
			`Chromium\Application\chrome.exe`,
			`Microsoft\Edge\Application\msedge.exe`,
		)...),
	}[runtime.GOOS]

	for _, path := range list {
		var err error
		found, err = exec.LookPath(path)
		has = err == nil
		if has {
			break
		}
	}

	return
}

// expandWindowsExePaths is a verbatim copy of the function from rod's
// browser.go.
//
// (c) MIT license: Copyright 2019 Yad Smood
func expandWindowsExePaths(list ...string) []string {
	newList := []string{}
	for _, p := range list {
		newList = append(
			newList,
			filepath.Join(os.Getenv("ProgramFiles"), p),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), p),
			filepath.Join(os.Getenv("LocalAppData"), p),
		)
	}

	return newList
}
