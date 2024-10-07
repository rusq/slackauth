package slackauth

import (
	"net/http"
	"net/http/httptest"
	reflect "reflect"
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

func Test_filterCookies(t *testing.T) {
	type args struct {
		cookies []*http.Cookie
	}
	tests := []struct {
		name string
		args args
		want []*http.Cookie
	}{
		{
			name: "Retains valid domains",
			args: args{
				cookies: []*http.Cookie{
					{Domain: ".slack.com"},
					{Domain: ".example.com"},
					{Domain: ".google.co.nz"},
					{Domain: ".google.com.au"},
					{Domain: ""},
					{Domain: ".endlessefforts.onelogin.com"},
				},
			},
			want: []*http.Cookie{
				{Domain: ".slack.com"},
				{Domain: ".google.co.nz"},
				{Domain: ".google.com.au"},
				{Domain: ".endlessefforts.onelogin.com"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterCookies(tt.args.cookies); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterCookies() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Benchmark_filterCookies(b *testing.B) {
	// generate a large list of slack cookies
	var cc = make([]*http.Cookie, 1_000_000)
	for i := range cc {
		cc[i] = &http.Cookie{
			Domain: ".slack.com",
		}
	}

	var res []*http.Cookie

	b.ResetTimer()
	for range b.N {
		res = filterCookies(cc)
	}
	_ = res
}
