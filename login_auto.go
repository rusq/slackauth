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
	"github.com/go-rod/rod/lib/launcher"
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
	}
	opts.apply(opt)

	isHeadless := !opts.debug
	l := launcher.New().
		Leakless(isLeaklessEnabled). // Causes false positive on Windows, see #260
		Headless(isHeadless).
		Devtools(false)

	url, err := l.Context(ctx).Launch()
	if err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "launch"}
	}
	defer l.Cleanup()

	var delay time.Duration = 0
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
		slog.Debug("looks like we're on the unknown browser page")
		code, err := opts.codeFn(email)
		if err != nil {
			return fmt.Errorf("failed to get challenge code: %w", err)
		}
		if err := enterCode(page, code); err != nil {
			return ErrBrowser{Err: err, FailedTo: "enter challenge code"}
		}
		return nil
	}).Element(idRedirect) // success
	if _, err := rctx.Do(); err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "wait for login to complete"}
	}

	ctx, cancel := context.WithTimeoutCause(ctx, 30*time.Second, errors.New("login timeout"))
	defer cancel()

	ctx, cancelCause := withTabGuard(ctx, browser, page.TargetID)
	defer cancelCause(nil)

	token, err := h.Wait(ctx)
	if err != nil {
		return "", nil, err
	}
	cookies, err := extractCookies(browser)
	if err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "extract cookies"}
	}

	return token, cookies, nil
}

func extractCookies(browser *rod.Browser) ([]*http.Cookie, error) {
	cook, err := browser.GetCookies()
	if err != nil {
		return nil, err
	}
	var cookies = make([]*http.Cookie, 0, len(cook))
	for _, c := range cook {
		cookies = append(cookies, &http.Cookie{
			Name:    c.Name,
			Value:   c.Value,
			Domain:  c.Domain,
			Path:    c.Path,
			Expires: c.Expires.Time(),
		})
	}
	return cookies, nil
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

func enterCode(page *rod.Page, code int) error {
	sCode := fmt.Sprintf("%06d", code)

	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf(idDigitN, i)
		el, err := page.Element(id)
		if err != nil {
			return ErrBrowser{Err: err, FailedTo: "find digit input field"}
		}
		if err := el.Input(string(sCode[i])); err != nil {
			return ErrBrowser{Err: err, FailedTo: "fill in digit input field"}
		}
	}
	return nil
}
