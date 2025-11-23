package slackauth

import (
	"context"
	"errors"
	"net/http"
	"runtime/trace"
	"strings"

	"github.com/rusq/slackauth/internal/qrslack"
)

var ErrLinkExpired = errors.New("login link expired")

func (c *Client) QRAuth(ctx context.Context, imageData string) (string, []*http.Cookie, error) {
	ctx, task := trace.NewTask(ctx, "QRAuth")
	defer task.End()

	loginURL, err := qrslack.Decode(imageData)
	if err != nil {
		return "", nil, err
	}

	browser, err := c.startBrowser(ctx)
	if err != nil {
		return "", nil, err
	}
	page, h, err := c.blankPage(ctx, browser)
	if err != nil {
		return "", nil, err
	}

	if err := c.openURL(ctx, page, loginURL); err != nil {
		return "", nil, err
	}

	ctx, cancel := withTabGuard(ctx, browser, page.TargetID, c.opts.lg)
	defer cancel(nil)

	// trap the redirect page and click it, if it appears.
	_, stopTrap := c.trapRedirect(ctx, page)
	defer stopTrap(errors.New("login finished"))

	title := page.MustElement("title").MustEval(`() => this.innerText`).String()
	if strings.Contains(title, "Link expired") {
		return "", nil, ErrLinkExpired
	}

	// blocks till it sees the token
	token, err := h.Token(ctx)
	if err != nil {
		return "", nil, err
	}

	var cookies []*http.Cookie
	cookies, err = convertCookies(browser.GetCookies())
	if c.opts.forceUser {
		// we need not store all cookies from the user browser.
		trace.WithRegion(ctx, "filterCookies", func() {
			cookies = filterCookies(cookies)
		})
	}
	if err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "extract cookies"}
	}

	return token, cookies, nil
}
