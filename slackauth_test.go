package slackauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_checkWorkspaceURL(t *testing.T) {
	t.Parallel()
	t.Run("error", func(t *testing.T) {
		t.Parallel()
		err := checkWorkspaceURL("http://127.0.0.1:9999")
		assert.ErrorIs(t, err, ErrWorkspaceNotFound)
	})
	t.Run("404", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()
		err := checkWorkspaceURL(srv.URL)
		assert.ErrorIs(t, err, ErrWorkspaceNotFound)
	})
	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		err := checkWorkspaceURL(srv.URL)
		assert.NoError(t, err)
	})
}

func Test_isURLSafe(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "empty",
			args: args{s: ""},
			want: true,
		},
		{
			name: "safe",
			args: args{s: "abcABC123-_.~"},
			want: true,
		},
		{
			name: "unsafe",
			args: args{s: "abcABC123-_.~!@#$%^&*()"},
			want: false,
		},
		{
			name: "unicode",
			args: args{s: "🍌"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isURLSafe(tt.args.s); got != tt.want {
				t.Errorf("isURLSafe() = %v, want %v", got, tt.want)
			}
		})
	}
}
