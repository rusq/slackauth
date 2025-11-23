package slackauth

import (
	"context"
	"fmt"
	"net/http"
	"runtime/trace"
	"strings"

	"github.com/go-rod/rod"
)

// hijacker is a contraption to hijack the request holding the token. Once the
// request is captured, token value is extracted and sent on the credsC
// channel.  Caller may retrieve it by calling [Token] method.
type hijacker struct {
	r      *rod.HijackRouter
	credsC chan creds
	lg     Logger
}

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

func (h *hijacker) hook(rh *rod.Hijack) {
	h.lg.Debug("hijack api.features")

	r := rh.Request.Req()

	token, err := extractToken(r)
	if err != nil {
		h.credsC <- creds{Err: fmt.Errorf("error parsing token out of request: %v", err)}
		return
	}

	h.credsC <- creds{Token: token}
}

// Stop terminates the hijacker and disables request hooks.
func (h *hijacker) Stop() error {
	defer close(h.credsC)
	if err := h.r.Stop(); err != nil {
		return err
	}
	return nil
}

// Token returns the token value or an error.  If the token has not yet been
// captured, it blocks until hijacker has captured the token value.
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
	maxFormParseMem = 131072  // maximum memory for the multipart form parser
	paramToken      = "token" // token form field name
)

func extractToken(r *http.Request) (string, error) {
	if err := r.ParseMultipartForm(maxFormParseMem); err != nil {
		return "", fmt.Errorf("error parsing request: %w", err)
	}
	tok := strings.TrimSpace(r.Form.Get(paramToken))
	if len(tok) == 0 {
		return "", fmt.Errorf("token not found in the form request")
	}
	return r.Form.Get(paramToken), nil
}
