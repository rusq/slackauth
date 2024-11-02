package slackauth

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_options_browserPath(t *testing.T) {
	// create a fake browser
	tmp := t.TempDir()
	fakeChromeBin := filepath.Join(tmp, "chrome")
	if err := os.WriteFile(fakeChromeBin, nil, 0o755); err != nil {
		t.Fatal(err)
	}

	lookPathBin, lookPathOk := lookPath() // whatever is discovered here is fine

	type fields struct {
		cookies         []*http.Cookie
		userAgent       string
		autoTimeout     time.Duration
		forceUser       bool
		useBundledBrwsr bool
		localBrowser    string
		codeFn          func(email string) (code int, err error)
		debug           bool
		lg              Logger
	}
	tests := []struct {
		name     string
		fields   fields
		wantPath string
		wantOk   bool
	}{
		{
			name: "bundled browser",
			fields: fields{
				useBundledBrwsr: true,
			},
			wantPath: "",
			wantOk:   false,
		},
		{
			name: "local browser",
			fields: fields{
				localBrowser: fakeChromeBin,
			},
			wantPath: fakeChromeBin,
			wantOk:   true,
		},
		{
			name:     "look path browser",
			fields:   fields{},
			wantPath: lookPathBin,
			wantOk:   lookPathOk,
		},
		{
			name: "force user overrides the use of bundled browser",
			fields: fields{
				useBundledBrwsr: true,
				forceUser:       true,
				localBrowser:    fakeChromeBin,
			},
			wantPath: fakeChromeBin,
			wantOk:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := options{
				cookies:         tt.fields.cookies,
				userAgent:       tt.fields.userAgent,
				autoTimeout:     tt.fields.autoTimeout,
				forceUser:       tt.fields.forceUser,
				useBundledBrwsr: tt.fields.useBundledBrwsr,
				localBrowser:    tt.fields.localBrowser,
				codeFn:          tt.fields.codeFn,
				debug:           tt.fields.debug,
				lg:              tt.fields.lg,
			}
			gotPath, gotOk := o.browserPath()
			if gotPath != tt.wantPath {
				t.Errorf("options.browserPath() gotPath = %v, want %v", gotPath, tt.wantPath)
			}
			if gotOk != tt.wantOk {
				t.Errorf("options.browserPath() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}

func TestRemoveBrowser(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "no error",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := RemoveBrowser(); (err != nil) != tt.wantErr {
				t.Errorf("RemoveBrowser() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
