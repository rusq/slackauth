package slackauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	idPassword      = "#password"
	idEmail         = "#email"
	idPasswordLogin = `[data-qa="sign_in_password_link"]`
	idAnyError      = `[data-qa-error="true"]`
	idRedirect      = `[data-qa="ssb_redirect_open_in_browser"]`

	debugDelay = 1 * time.Second
)

// Headless logs the user in headlessly, without opening the browser UI.  It
// is only suitable for user/email login method, as it does not require any
// additional user interaction.
func Headless(ctx context.Context, workspace, email, password string, opt ...Option) (string, []*http.Cookie, error) {
	wspURL, err := workspaceURL(workspace)
	if err != nil {
		return "", nil, err
	}
	var opts options
	opts.apply(opt)

	isHeadless := !opts.debug
	l := launcher.New().
		Leakless(false). // Causes false positive on Windows, see #260
		Headless(isHeadless).
		Devtools(false)
	defer l.Cleanup()

	url, err := l.Launch()
	if err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "launch"}
	}

	var delay = 0 * time.Millisecond
	if opts.debug {
		delay = debugDelay
	}

	var browser = rod.New().
		Context(ctx).
		ControlURL(url).
		Trace(opts.debug).
		SlowMotion(delay)
	defer browser.Close()

	if err := browser.Connect(); err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "connect"}
	}
	defer browser.Close()

	if err := setCookies(browser, opts.cookies); err != nil {
		return "", nil, err
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: wspURL})
	if err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "open page"}
	}

	h := newHijacker(page)
	defer h.Stop()

	// if there's no password element on the page, we must be on the "email
	// login" page.  We need to switch away to the password login.
	if has, _, err := page.Has(idPassword); err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "check for password field"}
	} else if !has {
		slog.Debug("switching to password login")
		el, err := page.Element(idPasswordLogin)
		if err != nil {
			return "", nil, ErrBrowser{Err: err, FailedTo: "find password login link"}
		}
		if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
			return "", nil, ErrBrowser{Err: err, FailedTo: "click password login link"}
		}
	}
	// fill in email and password fields.
	if fldEmail, err := page.Element(idEmail); err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "find email field"}
	} else {
		if err := fldEmail.Input(email); err != nil {
			return "", nil, ErrBrowser{Err: err, FailedTo: "fill in email field"}
		}
	}
	if fldPwd, err := page.Element(idPassword); err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "find password field"}
	} else {
		if err := fldPwd.Input(password); err != nil {
			return "", nil, ErrBrowser{Err: err, FailedTo: "fill in password field"}
		}
		if err := fldPwd.Type(input.Enter); err != nil {
			return "", nil, ErrBrowser{Err: err, FailedTo: "submit login form"}
		}
	}
	rctx := page.Race().Element(idAnyError).Handle(func(e *rod.Element) error {
		slog.Debug("looks like some error occurred")
		if opts.debug {
			page.MustScreenshot("login-error.png")
		}
		return errors.New("slack reported an error during login")
	}).Element(idRedirect)
	if _, err := rctx.Do(); err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "wait for login to complete"}
	}

	ctx, cancel := context.WithTimeoutCause(ctx, 30*time.Second, errors.New("login timeout"))
	defer cancel()

	ctx, cancelCause := withTabGuard(ctx, browser, page.TargetID)
	defer cancelCause(nil)

	token, cookies, err := h.Wait(ctx)
	if err != nil {
		return "", nil, err
	}
	return token, cookies, nil
}
