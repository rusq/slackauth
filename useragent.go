package slackauth

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"

// setUserAgent sets the user agent for the page.
func (o options) setUserAgent(page *rod.Page) error {
	if o.userAgent != "" {
		if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: o.userAgent}); err != nil {
			return err
		}
	}

	return nil
}
