package slackauth

import "github.com/go-rod/rod"

//go:generate mockgen -destination=wrappers_mocks_test.go -package=slackauth -source wrappers.go

// elementer is an interface for rod.Page.Element method. It is used for mocking
// rod.Page in tests.
type elementer interface {
	Element(selector string) (inputter, error)
}

type inputter interface {
	Input(string) error
}

type (
	pageWrapper rod.Page
)

func (p *pageWrapper) Element(selector string) (inputter, error) {
	el, err := (*rod.Page)(p).Element(selector)
	if err != nil {
		return nil, err
	}
	return el, nil
}
