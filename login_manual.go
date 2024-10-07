package slackauth

import (
	"context"
	"errors"
	"net/http"
	"runtime/trace"
)

// Browser is a function that initiates a login flow in a browser.
//
// Deprecated: Use [Client.Manual] instead.
var Browser = Manual

// Browser initiates a login flow in a browser (manual login).
//
// Deprecated: Use [Client.Manual] instead.
func Manual(ctx context.Context, workspace string, opt ...Option) (string, []*http.Cookie, error) {
	c, err := New(workspace, opt...)
	if err != nil {
		return "", nil, err
	}
	defer c.Close()

	return c.Manual(ctx)
}

// Manual initiates a login flow in a browser (manual login).
func (c *Client) Manual(ctx context.Context) (string, []*http.Cookie, error) {
	ctx, task := trace.NewTask(ctx, "Manual")
	defer task.End()

	browser, err := c.startBrowser(ctx)
	if err != nil {
		return "", nil, err
	}
	page, h, err := c.openSlackAuthTab(ctx, browser)
	if err != nil {
		return "", nil, err
	}

	ctx, cancel := withTabGuard(ctx, browser, page.TargetID, c.opts.lg)
	defer cancel(nil)

	// trap the redirect page and click it, if it appears.
	_, stopTrap := c.trapRedirect(ctx, page)
	defer stopTrap(errors.New("login finished"))

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
