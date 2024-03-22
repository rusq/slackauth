package slackauth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// hijacker is a helper for hijacking requests.
type hijacker struct {
	r      *rod.HijackRouter
	credsC chan creds
	lg     Logger
}

type creds struct {
	Token string
	Err   error
}

func newHijacker(page *rod.Page, lg Logger) *hijacker {
	var (
		r      = page.HijackRequests()
		credsC = make(chan creds, 1)
		hj     = &hijacker{r: r, credsC: credsC, lg: lg}
	)

	r.MustAdd(`*/api/api.features*`, func(h *rod.Hijack) {
		lg.Debug("hijack api.features")

		r := h.Request.Req()

		token, err := extractToken(r)
		if err != nil {
			credsC <- creds{Err: fmt.Errorf("error parsing token out of request: %v", err)}
			return
		}

		credsC <- creds{Token: token}
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

// Token waits for the hijacker to receive a token or an error.
func (h *hijacker) Token(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", context.Cause(ctx)
	case creds := <-h.credsC:
		return creds.Token, creds.Err
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

const (
	maxMem     = 131072
	paramToken = "token"
)

func extractToken(r *http.Request) (string, error) {
	if err := r.ParseMultipartForm(maxMem); err != nil {
		return "", err
	}
	return r.Form.Get(paramToken), nil
}
