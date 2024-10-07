package slackauth

import (
	"context"
	"fmt"
	"net/http"
	"runtime/trace"
	"strings"

	"github.com/go-rod/rod"
)

// hijacker is a helper for hijacking requests.
type hijacker struct {
	r      *rod.HijackRouter
	credsC chan creds
	lg     Logger
}

// creds holds token and an error, and is communicated through the credsC
// channel of the hijacker.
type creds struct {
	Token string
	Err   error
}

func newHijacker(ctx context.Context, page *rod.Page, lg Logger) (*hijacker, error) {
	hPg := page.Context(ctx)
	hj := &hijacker{
		r:      hPg.HijackRequests(),
		credsC: make(chan creds, 1),
		lg:     lg,
	}
	if err := hj.r.Add(`*/api/api.features*`, "", hj.hook); err != nil {
		return nil, fmt.Errorf("error adding hijack route: %w", err)
	}
	go hj.r.Run()
	lg.Debug("hijacker created")
	return hj, nil
}

func (hj *hijacker) hook(h *rod.Hijack) {
	hj.lg.Debug("hijack api.features")

	r := h.Request.Req()

	token, err := extractToken(r)
	if err != nil {
		hj.credsC <- creds{Err: fmt.Errorf("error parsing token out of request: %v", err)}
		return
	}

	hj.credsC <- creds{Token: token}
}

func (h *hijacker) Stop() error {
	defer close(h.credsC)
	if err := h.r.Stop(); err != nil {
		return err
	}
	return nil
}

// Token waits for the hijacker to receive a token or an error.
func (h *hijacker) Token(ctx context.Context) (string, error) {
	ctx, task := trace.NewTask(ctx, "Token")
	defer task.End()
	select {
	case <-ctx.Done():
		return "", context.Cause(ctx)
	case creds := <-h.credsC:
		return creds.Token, creds.Err
	}
}

const (
	maxMem     = 131072
	paramToken = "token"
)

func extractToken(r *http.Request) (string, error) {
	if err := r.ParseMultipartForm(maxMem); err != nil {
		return "", fmt.Errorf("error parsing request: %w", err)
	}
	tok := strings.TrimSpace(r.Form.Get(paramToken))
	if len(tok) == 0 {
		return "", fmt.Errorf("token not found in the form request")
	}
	return r.Form.Get(paramToken), nil
}
