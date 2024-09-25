package slackauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

const (
	idPasswordLogin = `[data-qa="sign_in_password_link"]`

	idPassword = "#password"
	idEmail    = "#email"

	idAnyError        = `[data-qa-error="true"]`
	idPasswordError   = "#password_error"
	idSignInAlertText = ".c-inline_alert__text"

	idRedirect = `[data-qa="ssb_redirect_open_in_browser"]`

	idUnknownBrowser = `#enter_code_app_root`
	idDigitN         = `[aria-label="digit %d of 6"]`

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
	if err := checkWorkspaceURL(wspURL); err != nil {
		return "", nil, err
	}

	var opts options = options{
		codeFn: SimpleChallengeFn,
		lg:     slog.Default(),
	}
	opts.apply(opt)

	isHeadless := !opts.debug

	l := browserLauncher(isHeadless)
	url, err := l.Context(ctx).Launch()
	if err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "launch"}
	}
	defer l.Cleanup()

	var delay time.Duration = 0
	if opts.debug {
		delay = debugDelay
	}

	browser := rod.New().
		Context(ctx).
		ControlURL(url).
		Trace(opts.debug).
		SlowMotion(delay)
	defer browser.Close()

	if err := browser.Connect(); err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "connect"}
	}

	if err := setCookies(browser, opts.cookies); err != nil {
		return "", nil, err
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: wspURL})
	if err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "open page"}
	}

	h := newHijacker(page, opts.lg)
	defer h.Stop()

	// if there's no password element on the page, we must be on the "email
	// login" page.  We need to switch away to the password login.
	if hasPwdField, _, err := page.Has(idPassword); err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "check for password field"}
	} else if !hasPwdField {
		opts.lg.Debug("switching to password login")
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
		opts.lg.Debug("looks like some error occurred")
		if has, _, err := page.Has(idPasswordError); err == nil && has {
			el, err := page.Element(idSignInAlertText)
			if err != nil {
				return ErrInvalidCredentials
			}
			txt, err := el.Text()
			if err != nil {
				return ErrInvalidCredentials
			}
			return fmt.Errorf("%w, slack message: [%s]", ErrInvalidCredentials, txt)
		}
		return ErrLoginError
	}).Element(idUnknownBrowser).Handle(func(e *rod.Element) error {
		opts.lg.Debug("looks like we're on the unknown browser page")
		code, err := opts.codeFn(email)
		if err != nil {
			return fmt.Errorf("failed to get challenge code: %w", err)
		}
		wrapped := (*pageWrapper)(page)
		if err := enterCode(wrapped, code); err != nil {
			return ErrBrowser{Err: err, FailedTo: "enter challenge code"}
		}
		return nil
	}).Element(idRedirect).Handle(func(e *rod.Element) error {
		opts.lg.Debug("looks like we're on the redirect page")
		page.Navigate(wspURL)
		return nil
	}) // success
	if _, err := rctx.Do(); err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "wait for login to complete"}
	}

	ctx, cancel := context.WithTimeoutCause(ctx, 30*time.Second, errors.New("login timeout"))
	defer cancel()

	ctx, cancelCause := withTabGuard(ctx, browser, page.TargetID, opts.lg)
	defer cancelCause(nil)

	token, err := h.Token(ctx)
	if err != nil {
		return "", nil, err
	}
	cookies, err := convertCookies(browser.GetCookies())
	if err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "extract cookies"}
	}

	return token, cookies, nil
}

// SimpleChallengeFn is a simple challenge function that reads a single
// integer from stdin.  It is used as a default challenge function when
// none is provided.
func SimpleChallengeFn(email string) (int, error) {
	var code int
	fmt.Printf("Slack has sent you an email message with a challenge code to your %s address.\nPlease open your email and type the code from the message.\n\nEnter code: ", email)
	_, err := fmt.Scanf("%d", &code)
	if err != nil {
		return 0, err
	}
	return code, nil
}

const codeLen = 6

func enterCode(page elementer, code int) error {
	if code > 999999 || code < 0 {
		return fmt.Errorf("code must be a 6-digit number, got %d", code)
	}
	sCode := fmt.Sprintf("%0*d", codeLen, code)

	for i := 1; i <= codeLen; i++ {
		id := fmt.Sprintf(idDigitN, i)
		el, err := page.Element(id)
		if err != nil {
			return ErrBrowser{Err: err, FailedTo: "find digit input field"}
		}
		if err := el.Input(string(sCode[i-1])); err != nil {
			return ErrBrowser{Err: err, FailedTo: "fill in digit input field"}
		}
	}

	return nil
}
