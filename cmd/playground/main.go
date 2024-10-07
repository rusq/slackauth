// Command playground is a manual testing tool.
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
	"runtime/trace"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/joho/godotenv"
	"github.com/rusq/chttp"
	"github.com/rusq/slackauth"
)

var _ = godotenv.Load()

var (
	auto      = flag.Bool("auto", false, "attempt auto login")
	forceNew  = flag.Bool("force-user", false, "force open a user browser, instead of the clean one")
	bundled   = flag.Bool("bundled", false, "force using a bundled browser")
	isDebug   = flag.Bool("d", os.Getenv("DEBUG") == "1", "enable debug")
	traceFile = flag.String("trace", "", "trace `filename`")
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

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}
func run(ctx context.Context) error {
	if *traceFile != "" {
		f, err := os.Create(*traceFile)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := trace.Start(f); err != nil {
			return err
		}
		defer trace.Stop()
	}

	c, err := initClient(envOrScan("AUTH_WORKSPACE", "Enter workspace: "), *isDebug)
	if err != nil {
		return err
	}
	defer c.Close()

	var (
		token   string
		cookies []*http.Cookie
	)
	if *auto {
		username := envOrScan("EMAIL", "Enter email: ")
		password := envOrScan("PASSWORD", "Enter password: ")
		token, cookies, err = autoLogin(ctx, c, username, password)
	} else {
		token, cookies, err = browserLogin(ctx, c)
	}
	if err != nil {
		return err
	}
	if token == "" {
		return errors.New("empty token")
	}
	if len(cookies) == 0 {
		return errors.New("empty cookies")
	}
	slog.Info("attempting slack login")
	if err := testCreds(ctx, token, cookies); err != nil {
		return err
	}
	slog.Info("login successful")
	return nil
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

func initClient(workspace string, trace bool) (*slackauth.Client, error) {
	var opts = []slackauth.Option{
		slackauth.WithNoConsentPrompt(),
		slackauth.WithDebug(trace),
	}
	if *forceNew {
		opts = append(opts, slackauth.WithForceUser())
	}
	if *bundled {
		opts = append(opts, slackauth.WithBundledBrowser())
	}

	c, err := slackauth.New(workspace, opts...)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func browserLogin(ctx context.Context, c *slackauth.Client) (string, []*http.Cookie, error) {
	ctx, task := trace.NewTask(ctx, "browserLogin")
	defer task.End()

	ctx, cancel := context.WithTimeoutCause(ctx, 180*time.Second, errors.New("user too slow"))
	defer cancel()
	token, cookies, err := c.Manual(ctx)
	if err != nil {
		return "", nil, err
	}
	fmt.Println(token)
	printCookies(cookies)
	return token, cookies, nil
}

func autoLogin(ctx context.Context, c *slackauth.Client, username, password string) (string, []*http.Cookie, error) {
	ctx, task := trace.NewTask(ctx, "autoLogin")
	defer task.End()
	ctx, cancel := context.WithTimeoutCause(ctx, 180*time.Second, errors.New("user too slow"))
	defer cancel()

	start := time.Now()
	token, cookies, err := c.Headless(ctx, username, password)
	if err != nil {
		return "", nil, err
	}
	slog.Info("login duration", "d", time.Since(start))
	fmt.Println(token)
	printCookies(cookies)
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
