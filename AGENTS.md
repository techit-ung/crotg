# Agent

After completing a task, update `TASK_BREAKDOWN.md` and `HANDOVER.md` to reflect the current status and next steps.

## Commands

- **Build**: `go build ./cmd/reviewer`
- **Run (TUI)**: `go run ./cmd/reviewer` (supports flags: `--base`, `--branch`, `--model`, `--guideline`, `--debug`)
- **Test All**: `go test ./...`
- **Test with Coverage**: `go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out`
- **Run Single Test/Package**: `go test ./internal/git` or `go test ./internal/git -run <TestName>`
- **Lint/Format**: `go fmt ./...` (or `gofmt -w .`)

## Code Style & Testing Patterns

- **Language**: Go (Go 1.25.6+, no Node.js/pnpm).
- **Naming**:
  - Standard Go conventions: `PascalCase` for exported, `camelCase` for internal.
  - Test names: `TestFunctionName_whenCondition_shouldResult` (e.g., `TestParseUnifiedDiff_whenValidDiff_shouldParseFilesAndLines`).
- **Tests**:
  - Structure: Use comments for `// arrange`, `// act`, `// assert`.
  - No nested tests.
  - Place tests in `*_test.go` files alongside the source (e.g., `internal/git/diff_test.go`).
  - Target coverage: ≥80% for protocol/schema packages (`internal/review`, `internal/bitbucket`).
- **Security**: Never persist API keys/tokens. Read from environment variables:
  - `OPENROUTER_API_KEY`: Required for LLM reviews.
  - `BITBUCKET_TOKEN`: Required for Bitbucket publishing.

## High-Level Architecture

The project is an interactive Terminal UI (TUI) for reviewing Git diffs using LLMs and publishing results to Bitbucket.

### Key Packages
- `cmd/reviewer`: Entry point and TUI initialization.
- `internal/app`: Main [Bubble Tea](https://github.com/charmbracelet/bubbletea) model and state machine.
- `internal/git`: Git operations (shelling out to `git` CLI) and unified diff parsing.
- `internal/llm`: [OpenRouter](https://openrouter.ai/) API client with retry logic and JSON logging.
- `internal/review`: Core review engine; handles per-file chunking, prompt building, and parallel LLM orchestration.
- `internal/config`: Persisted configuration at `~/.config/reviewer/config.json`.
- `internal/bitbucket`: Bitbucket Cloud API client and markdown comment composition.
- `internal/logger`: Structured JSON logging for debug mode.

### Architecture Logic
- **TUI Framework**: Built on the [Charmbracelet](https://charmbracelet.com/) ecosystem (`bubbletea`, `bubbles`, `lipgloss`).
- **State Machine**: Transitions from `StateWizard` (config/setup) to `StateDashboard` (active review with 5 tabs: Diff, Comments, Verdict, Publish, Config).
- **Review Strategy**: Per-file chunking with concurrency limiting. Comments are deduplicated via stable hashing.
- **Git Integration**: Relies on system `git` availability rather than `go-git` for better performance and compatibility with complex diffs.

## Project Documents

- `PLAN.md`: Full product specification and data models.
- `AGENTS.md`: Detailed repository guidelines and coding standards.
- `HANDOVER.md`: Current implementation status and milestones.
- `TASK_BREAKDOWN.md`: Progress tracking (Milestones 1-8 done).

# Repository Guidelines

## Project Structure & Module Organization

The Go module lives at the repo root, with roadmap docs in `PLAN.md` and `TASK_BREAKDOWN.md`. Place the CLI entry point in `cmd/reviewer/main.go`, Bubble Tea state in `internal/app`, git/diff helpers in `internal/git`, review + OpenRouter logic in `internal/review`, and Bitbucket publishing code in `internal/bitbucket`. Store guideline templates in `.review/` and config samples under `assets/`. Keep tests beside their packages (`internal/git/git_test.go`) and share fixtures via `testdata/`.

## Build, Test, and Development Commands
- `go run ./cmd/reviewer --base main --branch feature/foo` runs the TUI against real diffs.
- `go build ./cmd/reviewer` produces the CLI binary (run before tagging releases).
- `go test ./...` runs unit tests; add `-run Diff` when focusing on a suite.
- `go test ./... -coverprofile=coverage.out` then `go tool cover -html=coverage.out` to inspect review coverage before publishing.

## Coding Style & Naming Conventions
Format code with `gofmt -w .` or `goimports`; CI will reject unformatted diffs. Stick to Go's default tab indentation and keep lines under ~120 chars so Bubble Tea panes stay readable. Exported identifiers use MixedCaps (`DiffFile`, `ReviewComment`), while internals stay lowerCamel (`renderStatusBar`). Use `pkg_function.go` naming (e.g., `diff_parser.go`) and centralize logging helpers instead of scattered `fmt.Println`.

## Testing Guidelines
Rely on Go's `testing` package plus `testscript` when git fixtures are needed. Name tests `Test<Thing>` (e.g., `TestParseUnifiedDiff`). Any parser or network client should include table-driven tests covering success, validation errors, and timeout handling. When touching TUI state transitions, add assertions around model updates to guard regressions. Target ≥80% coverage for packages that ship protocol contracts (`internal/review`, `internal/bitbucket`).

## Commit & Pull Request Guidelines
No git history exists yet, so adopt Conventional Commits (`feat: add diff parser`, `fix: guard nil verdict`). Keep subjects ≤72 chars and explain motivation plus verification steps in the body. Pull requests should link the relevant milestone checkbox from `TASK_BREAKDOWN.md`, describe testing (`go test ./...` output), and include screenshots or GIFs when UI flows change.

## Security & Configuration Tips
Never commit API keys; load `OPENROUTER_API_KEY` and `BITBUCKET_TOKEN` from the environment. Use `.envrc` or your shell profile instead of `.env`. Validate user-supplied branch names before executing git commands, and redact tokens in logs. Config files belong under `~/.config/reviewer/` per the XDG guidance outlined in `PLAN.md`.
