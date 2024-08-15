package slackauth

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_hijacker_Token(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		h := hijacker{
			credsC: make(chan creds, 1),
		}
		go func() {
			h.credsC <- creds{Token: "token"}
		}()
		token, err := h.Token(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if token != "token" {
			t.Errorf("unexpected token: %v", token)
		}
	})
	t.Run("cancelled context returns the cause", func(t *testing.T) {
		h := hijacker{
			credsC: make(chan creds, 1),
		}
		ctx, cancel := context.WithCancelCause(context.Background())
		e := errors.New("cause")
		cancel(e)
		token, err := h.Token(ctx)
		if err == nil {
			t.Error("expected an error")
		}
		if token != "" {
			t.Errorf("unexpected token: %v", token)
		}
		assert.ErrorIs(t, err, e)
	})
	t.Run("error in the credentials struct", func(t *testing.T) {
		h := hijacker{
			credsC: make(chan creds, 1),
		}
		e := errors.New("error")
		go func() {
			h.credsC <- creds{Err: e}
		}()
		token, err := h.Token(context.Background())
		if err == nil {
			t.Error("expected an error")
		}
		if token != "" {
			t.Errorf("unexpected token: %v", token)
		}
		assert.ErrorIs(t, err, e)
	})
}

func Test_extractToken(t *testing.T) {
	type args struct {
		r *http.Request
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "request with form data containing token",
			args: args{
				r: mkMultipartRequest(url.Values{paramToken: {"123"}}),
			},
			want:    "123",
			wantErr: false,
		},
		{
			name: "request with form data without token",
			args: args{
				r: mkMultipartRequest(url.Values{"somefield": {"123"}}),
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "request without form data",
			args: args{
				r: httptest.NewRequest(http.MethodPost, "http://example.com", nil),
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractToken(tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func mkMultipartRequest(v url.Values) *http.Request {
	buf := new(bytes.Buffer)
	w := multipart.NewWriter(buf)
	for k, vs := range v {
		for _, v := range vs {
			w.WriteField(k, v)
		}
	}
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "http://example.com", buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}
