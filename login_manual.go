package slackauth

import (
	"context"
	"net/http"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// Browser initiates a login flow in a browser.
func Browser(ctx context.Context, workspace string, opt ...Option) (string, []*http.Cookie, error) {
	wspURL, err := workspaceURL(workspace)
	if err != nil {
		return "", nil, err
	}
	if err := checkWorkspaceURL(wspURL); err != nil {
		return "", nil, err
	}

	var opts options
	opts.apply(opt)

	l := launcher.New().
		Headless(false).             // browser window must be visible
		Leakless(isLeaklessEnabled). // Causes false positive on Windows, see #260
		Devtools(false)

	url, err := l.Context(ctx).Launch()
	if err != nil {
		return "", nil, ErrBrowser{Err: err, FailedTo: "launch"}
	}
	defer l.Cleanup()

	browser := rod.New().Context(ctx).ControlURL(url)
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

	ctx, cancel := withTabGuard(ctx, browser, page.TargetID)
	defer cancel(nil)

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
