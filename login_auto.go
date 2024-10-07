package slackauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/trace"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

const (
	pathPwdSignin = "sign_in_with_password"

	idPasswordLogin = `[data-qa="sign_in_password_link"]`

	idPassword = "#password"
	idEmail    = "#email"

	idAnyError        = `[data-qa-error="true"]`
	idPasswordError   = "#password_error"
	idSignInAlertText = ".c-inline_alert__text"

	idRedirect = `[data-qa="ssb_redirect_open_in_browser"]`

	idUnknownBrowser = `#enter_code_app_root`
	idDigitN         = `[aria-label="digit %d of 6"]`

	idClientLoaded = `[data-qa="tab_rail_home_button"]`

	debugDelay = 1 * time.Second
)

// Headless logs the user in headlessly, without opening the browser UI.  It
// is only suitable for user/email login method, as it does not require any
// additional user interaction.
//
// Deprecated: Use [Client.Headless] instead.
func Headless(ctx context.Context, workspace, email, password string, opt ...Option) (string, []*http.Cookie, error) {
	c, err := New(workspace, opt...)
	if err != nil {
		return "", nil, err
	}
	defer c.Close()
	return c.Headless(ctx, email, password)
}

func (c *Client) Headless(ctx context.Context, email, password string) (string, []*http.Cookie, error) {
	ctx, task := trace.NewTask(ctx, "Headless")
	defer task.End()

	browser, err := c.startPuppet(ctx, !c.opts.debug)
	if err != nil {
		return "", nil, err
	}

	page, h, err := c.openSlackAuthTab(ctx, browser)
	if err != nil {
		return "", nil, err
	}

	if err := c.doAutoLogin(ctx, page, email, password); err != nil {
		return "", nil, err
	}

	ctx, cancel := context.WithTimeoutCause(ctx, c.opts.autoTimeout, errors.New("login timeout"))
	defer cancel()

	ctx, cancelCause := withTabGuard(ctx, browser, page.TargetID, c.opts.lg)
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

// enterCode enters the 6-digit code into the challenge code input fields.
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

// startPuppet starts a new browser instance and returns a handle to it.
func (c *Client) startPuppet(ctx context.Context, headless bool) (*rod.Browser, error) {
	ctx, task := trace.NewTask(ctx, "startPuppet")
	defer task.End()

	l := c.newBrwsrLauncher(headless)

	url, err := l.Context(ctx).Launch()
	if err != nil {
		return nil, ErrBrowser{Err: err, FailedTo: "launch, you may need to close your browser first"}
	}
	c.atClose(toerrfn(l.Cleanup))

	var delay time.Duration = 0
	if c.opts.debug {
		delay = debugDelay
	}

	browser := rod.New().
		Context(ctx).
		ControlURL(url).
		DefaultDevice(devices.Clear).
		Trace(c.opts.debug).
		SlowMotion(delay)
	c.atClose(browser.Close)

	if err := browser.Connect(); err != nil {
		return nil, ErrBrowser{Err: err, FailedTo: "connect"}
	}

	return browser, nil
}

// doAutoLogin performs the login process on the given page. It expects the
// page to point to the Slack workspace login page.
func (c *Client) doAutoLogin(ctx context.Context, page *rod.Page, email, password string) error {
	ctx, task := trace.NewTask(ctx, "doAutoLogin")
	defer task.End()

	page = page.Context(ctx)
	// ensure the page is loaded before starting fiddling with it.
	if err := page.WaitLoad(); err != nil {
		return ErrBrowser{Err: err, FailedTo: "wait for page to load"}
	}
	// if there's no password element on the page, we must be on the "email
	// login" page.  We need to switch away to the password login.
	if hasPwdField, _, err := page.Has(idPassword); err != nil {
		return ErrBrowser{Err: err, FailedTo: "check for password field"}
	} else if !hasPwdField {
		c.opts.lg.Debug("switching to password login")
		el, err := page.Element(idPasswordLogin)
		if err != nil {
			return ErrBrowser{Err: err, FailedTo: "find password login link"}
		}
		if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
			return ErrBrowser{Err: err, FailedTo: "click password login link"}
		}
	}
	// fill in email and password fields.
	if fldEmail, err := page.Element(idEmail); err != nil {
		return ErrBrowser{Err: err, FailedTo: "find email field"}
	} else {
		if err := fldEmail.Input(email); err != nil {
			return ErrBrowser{Err: err, FailedTo: "fill in email field"}
		}
	}
	if fldPwd, err := page.Element(idPassword); err != nil {
		return ErrBrowser{Err: err, FailedTo: "find password field"}
	} else {
		if err := fldPwd.Input(password); err != nil {
			return ErrBrowser{Err: err, FailedTo: "fill in password field"}
		}
		if err := fldPwd.Type(input.Enter); err != nil {
			return ErrBrowser{Err: err, FailedTo: "submit login form"}
		}
	}
	rctx := page.Race().Element(idAnyError).Handle(func(e *rod.Element) error {
		rgn := trace.StartRegion(page.GetContext(), "idAnyError")
		defer rgn.End()
		c.opts.lg.Debug("looks like some error occurred")
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
		rgn := trace.StartRegion(page.GetContext(), "idUnknownBrowser")
		defer rgn.End()
		c.opts.lg.Debug("looks like we're on the unknown browser page")
		code, err := c.opts.codeFn(email)
		if err != nil {
			return fmt.Errorf("failed to get challenge code: %w", err)
		}
		wrapped := (*pageWrapper)(page)
		if err := enterCode(wrapped, code); err != nil {
			return ErrBrowser{Err: err, FailedTo: "enter challenge code"}
		}
		return nil
	}).Element(idRedirect).Handle(click) // success
	if _, err := rctx.Do(); err != nil {
		return ErrBrowser{Err: err, FailedTo: "wait for login to complete"}
	}
	return nil
}
