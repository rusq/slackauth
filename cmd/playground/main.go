package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/joho/godotenv"
	"github.com/rusq/chttp"
	"github.com/rusq/slackauth"
)

var enableTrace = os.Getenv("DEBUG") == "1"

var _ = godotenv.Load()

var (
	auto     = flag.Bool("auto", false, "attempt auto login")
	forceNew = flag.Bool("force-user", false, "force open a user browser, instead of the clean one")
	bundled  = flag.Bool("bundled", false, "force using a bundled browser")
)

type testResponse struct {
	AuthTestResponse
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type AuthTestResponse struct {
	URL    string `json:"url"`
	Team   string `json:"team"`
	User   string `json:"user"`
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
	// EnterpriseID is only returned when an enterprise id present
	EnterpriseID string `json:"enterprise_id,omitempty"`
	BotID        string `json:"bot_id"`
}

func main() {
	flag.Parse()
	slog.SetLogLoggerLevel(slog.LevelDebug)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	//browserLogin(ctx)

	var (
		token   string
		cookies []*http.Cookie
		err     error
	)
	if *auto {
		token, cookies, err = autoLogin(ctx)
	} else {
		token, cookies, err = browserLogin(ctx)
	}
	if err != nil {
		log.Fatal(err)
	}
	if token == "" {
		log.Fatal("empty token")
	}
	if len(cookies) == 0 {
		log.Fatal("empty cookies")
	}
	slog.Info("attempting slack login")
	if err := testCreds(ctx, token, cookies); err != nil {
		log.Fatal(err)
	}
	slog.Info("login successful")
}

func testCreds(ctx context.Context, token string, cookies []*http.Cookie) error {
	chc, err := chttp.New("https://slack.com", cookies)
	if err != nil {
		return err
	}
	var values = url.Values{
		"token": {token},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/auth.test", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := chc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	var tr testResponse
	if err := dec.Decode(&tr); err != nil {
		return err
	}
	if !tr.OK {
		return fmt.Errorf("API error: %s", tr.Error)
	}
	fmt.Printf("%#v\n", tr)

	return nil
}

func browserLogin(ctx context.Context) (string, []*http.Cookie, error) {
	workspace := envOrScan("AUTH_WORKSPACE", "Enter workspace: ")
	ctx, cancel := context.WithTimeoutCause(ctx, 180*time.Second, errors.New("user too slow"))
	defer cancel()

	var opts = []slackauth.Option{
		slackauth.WithNoConsentPrompt(),
	}
	if *forceNew {
		opts = append(opts, slackauth.WithForceUser())
	}
	if *bundled {
		opts = append(opts, slackauth.WithBundledBrowser())
	}

	c, err := slackauth.New(workspace, opts...)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	token, cookies, err := c.Manual(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(token)
	printCookies(cookies)
	return token, cookies, nil
}

func autoLogin(ctx context.Context) (string, []*http.Cookie, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, 180*time.Second, errors.New("user too slow"))
	defer cancel()

	workspace := envOrScan("AUTH_WORKSPACE", "Enter workspace: ")
	username := envOrScan("EMAIL", "Enter email: ")
	password := envOrScan("PASSWORD", "Enter password: ")

	c, err := slackauth.New(workspace, slackauth.WithDebug(enableTrace), slackauth.WithNoConsentPrompt())
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	start := time.Now()
	token, cookies, err := c.Headless(ctx, username, password)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(token)
	printCookies(cookies)
	slog.Info("login duration", "d", time.Since(start))
	return token, cookies, nil
}

func printCookies(cookies []*http.Cookie) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	defer tw.Flush()
	fmt.Fprintln(tw, "Domain\tName")
	for _, c := range cookies {
		fmt.Fprintf(tw, "%s\t%s\n", c.Domain, c.Name)
	}
}

func envOrScan(env, prompt string) string {
	v := os.Getenv(env)
	if v != "" {
		return v
	}
	for v == "" {
		fmt.Print(prompt)
		fmt.Scanln(&v)
	}
	return v
}
