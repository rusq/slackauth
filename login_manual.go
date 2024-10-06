package slackauth

import (
	"context"
	"net/http"
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
	browser, err := c.startBrowser(ctx)
	if err != nil {
		return "", nil, err
	}
	page, err := c.navigate(browser)
	if err != nil {
		return "", nil, err
	}

	h := newHijacker(page, c.opts.lg)
	c.atClose(h.Stop)

	ctx, cancel := withTabGuard(ctx, browser, page.TargetID, c.opts.lg)
	defer cancel(nil)

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
