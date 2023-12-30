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
	debug   bool
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

// WithDebug enables debug logging.
func WithDebug() Option {
	return func(o *options) {
		o.debug = true
	}
}

var (
	// ErrLoginTimeout indicates that there was an error during the post-login
	// flow.
	ErrLoginTimeout = errors.New("login timeout")
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
