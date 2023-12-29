package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/joho/godotenv"
)

var _ = godotenv.Load()

var (
	email        = os.Getenv("EMAIL")
	correctPwd   = os.Getenv("PASSWORD")
	incorrectPwd = "123"
	passwd       = correctPwd
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	if err := userLogin(ctx); err != nil {
		log.Fatal(err)
	}
}

func userLogin(ctx context.Context) error {
	l := launcher.New().
		Headless(false).
		Devtools(false)
	defer l.Cleanup()

	url := l.MustLaunch()

	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("https://ora600.slack.com/")

	h := newHijacker(page)
	defer h.Stop()

	token, cookies, err := h.Wait(ctx)
	if err != nil {
		return err
	}
	fmt.Println(token)
	fmt.Println(cookies)
	return nil
}

func headless() error {
	// Headless runs the browser on foreground, you can also use flag "-rod=show"
	// Devtools opens the tab in each new tab opened automatically
	l := launcher.New().
		Headless(false).
		Devtools(true)

	defer l.Cleanup()

	url := l.MustLaunch()

	// Trace shows verbose debug information for each action executed
	// SlowMotion is a debug related function that waits 2 seconds between
	// each action, making it easier to inspect what your code is doing.
	browser := rod.New().
		ControlURL(url).
		Trace(true).
		// SlowMotion(500 * time.Millisecond).
		MustConnect()

	// ServeMonitor plays screenshots of each tab. This feature is extremely
	// useful when debugging with headless mode.
	// You can also enable it with flag "-rod=monitor"
	launcher.Open(browser.ServeMonitor(""))

	defer browser.MustClose()

	page := browser.MustPage("https://ora600.slack.com/")

	h := newHijacker(page)
	defer h.Stop()

	// if there's no password element on the page, we must be on the "email
	// login" page.  We need to switch away to the password login.
	if !page.MustHas("#password") {
		log.Println("switching to password login")
		page.MustElement(`[data-qa="sign_in_password_link"]`).MustClick()
	}
	// fill in email and password fields.
	page.MustElement("#email").MustInput(email)
	page.MustElement("#password").MustInput(passwd).MustType(input.Enter)
	var errorOccurred bool
	_ = page.Race().Element(`[data-qa-error="true"]`).MustHandle(func(e *rod.Element) {
		log.Println("other error")
		errorOccurred = true
	}).Element(`[data-qa="ssb_redirect_open_in_browser"]`).MustDo()

	if errorOccurred {
		return errors.New("login error")
	}

	// // text := page.MustElement(".codesearch-results p").MustText()

	// fmt.Println(text)

	// utils.Pause() // pause goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	token, cookies, err := h.Wait(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println(token)
	fmt.Println(cookies)
	return nil
}

func extractToken(r *http.Request) (string, error) {
	if err := r.ParseMultipartForm(131072); err != nil {
		return "", err
	}
	return r.Form.Get("token"), nil
}

type hijacker struct {
	r      *rod.HijackRouter
	credsC chan creds
}

type creds struct {
	Token   string
	Cookies []*http.Cookie
	Err     error
}

func newHijacker(page *rod.Page) *hijacker {
	var (
		r      = page.HijackRequests()
		credsC = make(chan creds, 1)
		hj     = &hijacker{r: r, credsC: credsC}
	)
	r.MustAdd(`*/api/api.features*`, func(h *rod.Hijack) {
		log.Println("hijack api.features")

		r := h.Request.Req()

		log.Printf("REQUEST: %#v", r)

		token, err := extractToken(r)
		if err != nil {
			credsC <- creds{Err: fmt.Errorf("error parsing token out of request: %v", err)}
			return
		}

		cookies := r.Cookies()
		log.Printf("TOKEN: %q", token)
		log.Printf("COOKIES: %v", cookies)

		h.LoadResponse(http.DefaultClient, true)

		credsC <- creds{Token: token, Cookies: cookies}
	})
	go r.Run()
	return hj
}

func (h *hijacker) Stop() error {
	if err := h.r.Stop(); err != nil {
		return err
	}

	close(h.credsC)

	return nil
}

func (h *hijacker) Wait(ctx context.Context) (string, []*http.Cookie, error) {
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case creds := <-h.credsC:
		return creds.Token, creds.Cookies, creds.Err
	}
}
