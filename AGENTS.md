# Repository Guidelines

## Project Structure & Module Organization
The Go module lives at the repo root, with roadmap docs in `PLAN.md` and `TAKS_BREAKDOWN.md`. Place the CLI entry point in `cmd/reviewer/main.go`, Bubble Tea state in `internal/tui`, git/diff helpers in `internal/git`, review + OpenRouter logic in `internal/review`, and Bitbucket publishing code in `internal/publish`. Store guideline templates in `.review/` and config samples under `assets/`. Keep tests beside their packages (`internal/git/git_test.go`) and share fixtures via `testdata/`.

## Build, Test, and Development Commands
- `go run ./cmd/reviewer --base main --branch feature/foo` runs the TUI against real diffs.
- `go build ./cmd/reviewer` produces the CLI binary (run before tagging releases).
- `go test ./...` runs unit tests; add `-run Diff` when focusing on a suite.
- `go test ./... -coverprofile=coverage.out` then `go tool cover -html=coverage.out` to inspect review coverage before publishing.

## Coding Style & Naming Conventions
Format code with `gofmt -w .` or `goimports`; CI will reject unformatted diffs. Stick to Go's default tab indentation and keep lines under ~120 chars so Bubble Tea panes stay readable. Exported identifiers use MixedCaps (`DiffFile`, `ReviewComment`), while internals stay lowerCamel (`renderStatusBar`). Use `pkg_function.go` naming (e.g., `diff_parser.go`) and centralize logging helpers instead of scattered `fmt.Println`.

## Testing Guidelines
Rely on Go's `testing` package plus `testscript` when git fixtures are needed. Name tests `Test<Thing>` (e.g., `TestParseUnifiedDiff`). Any parser or network client should include table-driven tests covering success, validation errors, and timeout handling. When touching TUI state transitions, add assertions around model updates to guard regressions. Target ≥80% coverage for packages that ship protocol contracts (`internal/review`, `internal/publish`).

## Commit & Pull Request Guidelines
No git history exists yet, so adopt Conventional Commits (`feat: add diff parser`, `fix: guard nil verdict`). Keep subjects ≤72 chars and explain motivation plus verification steps in the body. Pull requests should link the relevant milestone checkbox from `TAKS_BREAKDOWN.md`, describe testing (`go test ./...` output), and include screenshots or GIFs when UI flows change.
After completing a task, update `TAKS_BREAKDOWN.md` and `HANDOVER.md` to reflect the current status and next steps.

## Security & Configuration Tips
Never commit API keys; load `OPENROUTER_API_KEY` and `BITBUCKET_TOKEN` from the environment. Use `.envrc` or your shell profile instead of `.env`. Validate user-supplied branch names before executing git commands, and redact tokens in logs. Config files belong under `~/.config/reviewer/` per the XDG guidance outlined in `PLAN.md`.
