package slackauth

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// newBrwsrLauncher creates a new browser launcher with the given headless
// mode.
func (c *Client) newBrwsrLauncher(headless bool) *launcher.Launcher {
	l := launcher.New().Headless(headless).Leakless(isLeaklessEnabled).Devtools(false)
	if binpath, ok := lookPath(); !c.opts.useBundledBrwsr && ok {
		l = l.Bin(binpath)
	}
	return l
}

// usrBrwsrLauncher creates a new user-mode browser launcher.
func (c *Client) usrBrwsrLauncher() *launcher.Launcher {
	l := launcher.NewUserMode().Headless(false).Leakless(isLeaklessEnabled).Devtools(false)
	if binpath, ok := lookPath(); !c.opts.useBundledBrwsr && ok {
		l = l.Bin(binpath)
	}
	return l
}

// lookPath is extended launcher.LookPath that includes support for Brave
// browser.
//
// (c) MIT license: Copyright 2019 Yad Smood
func lookPath() (found string, has bool) {
	list := map[string][]string{
		"darwin": {
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/google-chrome",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		},
		"linux": {
			"brave-browser",
			"chrome",
			"google-chrome",
			"/usr/bin/google-chrome",
			"/usr/bin/brave-browser",
			"microsoft-edge",
			"/usr/bin/microsoft-edge",
			"chromium",
			"chromium-browser",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/data/data/com.termux/files/usr/bin/chromium-browser",
		},
		"openbsd": {
			"chrome",
			"chromium",
		},
		"windows": append([]string{"chrome", "edge"}, expandWindowsExePaths(
			`BraveSoftware\Brave-Browser\Application\brave.exe`,
			`Google\Chrome\Application\chrome.exe`,
			`Chromium\Application\chrome.exe`,
			`Microsoft\Edge\Application\msedge.exe`,
		)...),
	}[runtime.GOOS]

	for _, path := range list {
		var err error
		found, err = exec.LookPath(path)
		has = err == nil
		if has {
			break
		}
	}

	return
}

// expandWindowsExePaths is a verbatim copy of the function from rod's
// browser.go.
//
// (c) MIT license: Copyright 2019 Yad Smood
func expandWindowsExePaths(list ...string) []string {
	newList := []string{}
	for _, p := range list {
		newList = append(
			newList,
			filepath.Join(os.Getenv("ProgramFiles"), p),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), p),
			filepath.Join(os.Getenv("LocalAppData"), p),
		)
	}

	return newList
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
