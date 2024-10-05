package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/joho/godotenv"
	"github.com/rusq/slackauth"
)

var enableTrace = os.Getenv("DEBUG") == "1"

var _ = godotenv.Load()

var (
	auto = flag.Bool("auto", false, "auto login")
)

func main() {
	flag.Parse()
	slog.SetLogLoggerLevel(slog.LevelDebug)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	//browserLogin(ctx)
	if *auto {
		autoLogin(ctx)
	} else {
		browserLogin(ctx)
	}
}

func browserLogin(ctx context.Context) {
	workspace := envOrScan("AUTH_WORKSPACE", "Enter workspace: ")
	ctx, cancel := context.WithTimeoutCause(ctx, 180*time.Second, errors.New("user too slow"))
	defer cancel()

	token, cookies, err := slackauth.Browser(ctx, workspace, slackauth.WithNoConsentPrompt(), slackauth.WithUserAgentAuto())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(token)
	fmt.Println(cookies)
}

func autoLogin(ctx context.Context) {
	ctx, cancel := context.WithTimeoutCause(ctx, 180*time.Second, errors.New("user too slow"))
	defer cancel()

	workspace := envOrScan("AUTH_WORKSPACE", "Enter workspace: ")
	username := envOrScan("EMAIL", "Enter email: ")
	password := envOrScan("PASSWORD", "Enter password: ")

	token, cookies, err := slackauth.Headless(ctx, workspace, username, password, slackauth.WithDebug(enableTrace), slackauth.WithNoConsentPrompt(), slackauth.WithUserAgentAuto())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(token)
	fmt.Println(cookies)
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
