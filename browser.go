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

// newBrwsrLauncher creates a new incognito browser launcher with the given
// headless mode.
func (c *Client) newBrwsrLauncher(headless bool) *launcher.Launcher {
	l := launcher.New().Headless(headless).Leakless(isLeaklessEnabled).Devtools(false)
	if binpath, ok := c.opts.browserPath(); ok {
		l = l.Bin(binpath)
	}
	return l
}

// usrBrwsrLauncher creates a new user-mode browser launcher.
func (c *Client) usrBrwsrLauncher() *launcher.Launcher {
	l := launcher.NewUserMode().Headless(false).Leakless(isLeaklessEnabled).Devtools(false)
	if binpath, ok := c.opts.browserPath(); ok {
		l = l.Bin(binpath)
	}
	return l
}

// browserPath returns the path to the browser executable and a boolean
// indicating whether the path is valid.
func (o options) browserPath() (path string, ok bool) {
	if o.useBundledBrwsr && !o.forceUser {
		// bundled browser can't operate in the user mode.  forceUser overrides
		// useBundledBrwsr.
		return "", false
	}
	if o.localBrowser != "" {
		if p, err := exec.LookPath(o.localBrowser); err == nil {
			return p, true
		}
	}
	return lookPath()
}

var ErrNoBrowsers = fmt.Errorf("no browsers found")

// ListBrowsers returns a list of browsers that are installed on the system.
func ListBrowsers() ([]LocalBrowser, error) {
	LocalBrowsers, ok := discover()
	if !ok {
		return nil, ErrNoBrowsers
	}
	return LocalBrowsers, nil
}

const (
	bChrome   = "Google Chrome"
	bChromium = "Chromium"
	bEdge     = "Microsoft Edge"
	bBrave    = "Brave"
)

type LocalBrowser struct {
	Name string
	Path string
}

// lookPath is extended launcher.LookPath that includes support for Brave
// browser.
//
// (c) MIT license: Copyright 2019 Yad Smood
func discover() (found []LocalBrowser, has bool) {
	list := map[string][]LocalBrowser{
		"darwin": {
			{bBrave, "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"},
			{bEdge, "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"},
			{bChrome, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
			{bChromium, "/Applications/Chromium.app/Contents/MacOS/Chromium"},
			{bChrome, "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary"},
			{bChrome, "/usr/bin/google-chrome-stable"},
			{bChrome, "/usr/bin/google-chrome"},
			{bChromium, "/usr/bin/chromium"},
			{bChromium, "/usr/bin/chromium-browser"},
		},
		"linux": {
			{bChrome, "brave-browser"},
			{bChrome, "chrome"},
			{bChrome, "google-chrome"},
			{bChrome, "/usr/bin/google-chrome"},
			{bChrome, "/usr/bin/brave-browser"},
			{bChrome, "microsoft-edge"},
			{bChrome, "/usr/bin/microsoft-edge"},
			{bChrome, "chromium"},
			{bChrome, "chromium-browser"},
			{bChrome, "/usr/bin/google-chrome-stable"},
			{bChrome, "/usr/bin/chromium"},
			{bChrome, "/usr/bin/chromium-browser"},
			{bChrome, "/snap/bin/chromium"},
			{bChrome, "/data/data/com.termux/files/usr/bin/chromium-browser"},
		},
		"openbsd": {
			{bChrome, "chrome"},
			{bChrome, "chromium"},
		},
		"windows": append(
			[]LocalBrowser{{bEdge, "edge"}, {bBrave, "brave"}, {bChrome, "chrome"}},
			expandWindowsExePathsX(
				LocalBrowser{bEdge, `Microsoft\Edge\Application\msedge.exe`},
				LocalBrowser{bEdge, `BraveSoftware\Brave-Browser\Application\brave.exe`},
				LocalBrowser{bEdge, `Google\Chrome\Application\chrome.exe`},
				LocalBrowser{bEdge, `Chromium\Application\chrome.exe`},
			)...),
	}[runtime.GOOS]

	for _, br := range list {
		var err error
		p, err := exec.LookPath(br.Path)
		if err == nil {
			found = append(found, LocalBrowser{br.Name, p})
		}
	}

	return found, len(found) > 0
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
			`Microsoft\Edge\Application\msedge.exe`,
			`BraveSoftware\Brave-Browser\Application\brave.exe`,
			`Google\Chrome\Application\chrome.exe`,
			`Chromium\Application\chrome.exe`,
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

// expandWindowsExePaths is a verbatim copy of the function from rod's
// browser.go.
//
// (c) MIT license: Copyright 2019 Yad Smood
func expandWindowsExePathsX(list ...LocalBrowser) []LocalBrowser {
	newList := []LocalBrowser{}
	for _, p := range list {
		newList = append(
			newList,
			LocalBrowser{p.Name, filepath.Join(os.Getenv("ProgramFiles"), p.Path)},
			LocalBrowser{p.Name, filepath.Join(os.Getenv("ProgramFiles(x86)"), p.Path)},
			LocalBrowser{p.Name, filepath.Join(os.Getenv("LocalAppData"), p.Path)},
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

// RemveBundled removes the bundled browser from the system.
func RemoveBrowser() error {
	bpath := launcher.DefaultBrowserDir
	if bpath == "" {
		return nil
	}
	if _, err := os.Stat(bpath); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(bpath); err != nil {
		return fmt.Errorf("failed to remove bundled browser: %w", err)
	}
	return nil
}
