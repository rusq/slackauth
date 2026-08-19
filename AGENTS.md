# Repository Guidelines

## Project Structure & Module Organization
This repository is a small Go module centered on the `slackauth` package. Most library code lives at the repo root in files such as `slackauth.go`, `login_auto.go`, and `browser.go`. Internal-only helpers live under `internal/qrslack/`. The manual test harness is `cmd/playground/main.go`, and shell-based experimentation lives in `slack_oauth_example.sh`. Unit tests sit next to the code they cover as `*_test.go`.

## Build, Test, and Development Commands
Use the standard Go toolchain:

- `go build ./...`: build the library and playground binary.
- `go test ./...`: run the full test suite locally.
- `go test -v ./...`: match CI verbosity from `.github/workflows/go.yml`.
- `go run ./cmd/playground`: start the manual playground for interactive auth flows.
- `make play`: wrapper around the playground entrypoint.
- `make manual_tests`: run tests, then exercise several playground modes manually.

CI currently runs `go build -v ./...` and `go test -v ./...` on Go 1.23. The module itself targets Go 1.22 in `go.mod`.

## Coding Style & Naming Conventions
Follow normal Go conventions: format with `gofmt`, keep imports gofmt-sorted, and use tabs for indentation. Keep exported identifiers in `CamelCase`, unexported helpers in `camelCase`, and test names in the `TestXxx` / `BenchmarkXxx` form. Preserve the existing file pattern of grouping related behavior by concern, for example `login_manual.go` and `login_auto.go`.

Generated files should stay generated: `wrappers_mocks_test.go` is produced by `mockgen`, so regenerate it instead of hand-editing it.

## Testing Guidelines
Tests use Go’s `testing` package, with `stretchr/testify/assert` and `go.uber.org/mock` where useful. Prefer table-driven tests for parsing, filtering, and login edge cases. Keep fast unit tests next to the package they exercise; reserve `cmd/playground` for manual verification of browser-driven flows.

## Commit & Pull Request Guidelines
Recent history favors short, imperative commit subjects such as `fix browser names` or `Implement Mobile link signin`. Keep subjects concise and action-oriented. For pull requests, include a clear summary, note any browser or Slack flow affected, link related issues, and mention how you verified the change (`go test ./...`, manual playground checks, or both).

## Security & Configuration Tips
Do not commit real Slack credentials, tokens, or `.env` secrets. The playground reads environment variables such as `AUTH_WORKSPACE`, `EMAIL`, and `PASSWORD`; keep those local only.
