package slackauth

import (
	"fmt"
	"runtime"

	"github.com/go-rod/rod/lib/proto"
)

var (
	defaultChromeVersion = "129.0.0.0"
	defaultWebkitVersion = "537.36"

	DefaultUserAgent = UserAgent(defaultWebkitVersion, defaultChromeVersion, "")
)

//go:generate mockgen -destination=useragent_mock_test.go -package=slackauth -source=useragent.go userAgentSetter
type userAgentSetter interface {
	SetUserAgent(req *proto.NetworkSetUserAgentOverride) error
}

// setUserAgent sets the user agent for the page.
func (o options) setUserAgent(page userAgentSetter) error {
	if o.userAgent != "" {
		if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: o.userAgent}); err != nil {
			return err
		}
	}

	return nil
}

// UserAgent returns a user agent string with the given WebKit and Chrome
// versions, and the OS.  If any of the versions are empty, the default
// versions are used.  If the OS is empty, the OS is determined from the
// runtime.GOOS.
func UserAgent(webkitVer, chromeVer, os string) string {
	if webkitVer == "" {
		webkitVer = defaultWebkitVersion
	}
	if chromeVer == "" {
		chromeVer = defaultChromeVersion
	}
	if os == "" {
		os = userAgentOS(runtime.GOOS)
	}

	return fmt.Sprintf(`Mozilla/5.0 (%[1]s) AppleWebKit/%[2]s (KHTML, like Gecko) Chrome/%[3]s Safari/%[2]s`, os, webkitVer, chromeVer)
}

func userAgentOS(goos string) string {
	switch goos {
	case "darwin":
		return "Macintosh; Intel Mac OS X 10_15_7"
	case "linux":
		return "X11; Linux x86_64"
	case "windows":
		return "Windows NT 10.0; Win64; x64"
	default:
		return "X11; Linux x86_64"
	}

}
