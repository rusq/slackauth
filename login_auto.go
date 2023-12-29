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
	idPassword = "#password"
	idEmail    = "#email"
	idAnyError = `[data-qa-error="true"]`
	idRedirect = `[data-qa="ssb_redirect_open_in_browser"]`

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

	var browser = rod.New().Context(ctx)
	if opts.debug {
		l := launcher.New().Headless(false).Devtools(false)
		url := l.MustLaunch()
		browser = browser.ControlURL(url).Trace(true).SlowMotion(debugDelay)
	}

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
	if !page.MustHas(idPassword) {
		slog.Debug("switching to password login")
		page.MustElement(`[data-qa="sign_in_password_link"]`).MustClick()
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
	var errorOccurred bool
	_ = page.Race().
		Element(idAnyError).
		MustHandle(func(e *rod.Element) {
			slog.Debug("looks like some error occurred")
			if opts.debug {
				page.MustScreenshot("login-error.png")
			}
			errorOccurred = true
		}).
		Element(idRedirect).MustDo()

	if errorOccurred {
		return "", nil, errors.New("login error")
	}

	ctx, cancel := context.WithTimeoutCause(ctx, 30*time.Second, errors.New("login timeout"))
	defer cancel()
	token, cookies, err := h.Wait(ctx)
	if err != nil {
		return "", nil, err
	}
	return token, cookies, nil
}
