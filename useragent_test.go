package slackauth

import (
	"errors"
	"net/http"
	"testing"

	"github.com/go-rod/rod/lib/proto"
	gomock "go.uber.org/mock/gomock"
)

func TestUserAgent(t *testing.T) {
	type args struct {
		webkitVer string
		chromeVer string
		os        string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "default",
			args: args{
				webkitVer: "",
				chromeVer: "",
				os:        "X11; Linux x86_64",
			},
			want: `Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36`,
		},
		{
			name: "some versions",
			args: args{
				webkitVer: "123.0.1.2",
				chromeVer: "456.7.8.9",
				os:        "Windows NT 10.0; Win64; x64",
			},
			want: `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/123.0.1.2 (KHTML, like Gecko) Chrome/456.7.8.9 Safari/123.0.1.2`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UserAgent(tt.args.webkitVer, tt.args.chromeVer, tt.args.os); got != tt.want {
				t.Errorf("UserAgent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_userAgentOS(t *testing.T) {
	type args struct {
		goos string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "darwin",
			args: args{
				goos: "darwin",
			},
			want: "Macintosh; Intel Mac OS X 10_15_7",
		},
		{
			name: "linux",
			args: args{
				goos: "linux",
			},
			want: "X11; Linux x86_64",
		},
		{
			name: "windows",
			args: args{
				goos: "windows",
			},
			want: "Windows NT 10.0; Win64; x64",
		},
		{
			name: "default",
			args: args{
				goos: "unknown",
			},
			want: "X11; Linux x86_64",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := userAgentOS(tt.args.goos); got != tt.want {
				t.Errorf("userAgentOS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_options_setUserAgent(t *testing.T) {
	type fields struct {
		cookies   []*http.Cookie
		userAgent string
		codeFn    func(email string) (code int, err error)
		debug     bool
		lg        Logger
	}
	tests := []struct {
		name    string
		fields  fields
		expect  func(*MockuserAgentSetter)
		wantErr bool
	}{
		{
			"no user agent",
			fields{
				userAgent: "",
			},
			func(m *MockuserAgentSetter) {
				// nothing
			},
			false,
		},
		{
			"user agent is set",
			fields{
				userAgent: "blah",
			},
			func(m *MockuserAgentSetter) {
				m.EXPECT().SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: "blah"}).Return(nil)
			},
			false,
		},
		{
			"error setting user agent",
			fields{
				userAgent: "blah",
			},
			func(m *MockuserAgentSetter) {
				m.EXPECT().SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: "blah"}).Return(errors.New("error"))
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := options{
				cookies:   tt.fields.cookies,
				userAgent: tt.fields.userAgent,
				codeFn:    tt.fields.codeFn,
				debug:     tt.fields.debug,
				lg:        tt.fields.lg,
			}
			ctrl := gomock.NewController(t)
			muas := NewMockuserAgentSetter(ctrl)
			tt.expect(muas)
			if err := o.setUserAgent(muas); (err != nil) != tt.wantErr {
				t.Errorf("options.setUserAgent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
