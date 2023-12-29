package slackauth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// hijacker is a helper for hijacking requests.
type hijacker struct {
	r      *rod.HijackRouter
	credsC chan creds
}

type creds struct {
	Token   string
	Cookies []*http.Cookie
	Err     error
}

func newHijacker(page *rod.Page) *hijacker {
	var (
		r      = page.HijackRequests()
		credsC = make(chan creds, 1)
		hj     = &hijacker{r: r, credsC: credsC}
	)
	r.MustAdd(`*/api/api.features*`, func(h *rod.Hijack) {
		slog.Debug("hijack api.features")

		r := h.Request.Req()

		slog.Debug("request", "request", fmt.Sprintf("%#v", r))

		token, err := extractToken(r)
		if err != nil {
			credsC <- creds{Err: fmt.Errorf("error parsing token out of request: %v", err)}
			return
		}

		cookies := r.Cookies()
		if err := h.LoadResponse(http.DefaultClient, true); err != nil {
			credsC <- creds{Err: fmt.Errorf("error loading response: %v", err)}
			return
		}

		credsC <- creds{Token: token, Cookies: cookies}
	})
	go r.Run()
	return hj
}

func (h *hijacker) Stop() error {
	if err := h.r.Stop(); err != nil {
		return err
	}

	close(h.credsC)

	return nil
}

func (h *hijacker) Wait(ctx context.Context) (string, []*http.Cookie, error) {
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case creds := <-h.credsC:
		return creds.Token, creds.Cookies, creds.Err
	}
}

func setCookies(browser *rod.Browser, cookies []*http.Cookie) error {
	if len(cookies) == 0 {
		return nil
	}
	for _, c := range cookies {
		if err := browser.SetCookies([]*proto.NetworkCookieParam{
			{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Expires: proto.TimeSinceEpoch(c.Expires.Unix())},
		}); err != nil {
			return fmt.Errorf("failed to set cookies: %w", err)
		}
	}
	return nil
}

const maxMem = 131072

func extractToken(r *http.Request) (string, error) {
	if err := r.ParseMultipartForm(maxMem); err != nil {
		return "", err
	}
	return r.Form.Get("token"), nil
}
