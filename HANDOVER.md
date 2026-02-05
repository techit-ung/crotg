# Handover

## Status
- Milestone 1 complete: Go module initialized, Bubble Tea scaffolded, config load/save, env helpers.
- Milestone 2 complete: repo detection, branch selection, diff generation/parsing, Diff tab UI.
- Milestone 3 complete: guideline scanning, multi-select, free-text input, and guideline hash display.
- Milestone 4 complete: OpenRouter client, prompt builders, per-file review chunking, comment dedupe, verdict generation.
- Milestone 5 complete: comments table with filters, detail pane, and publish toggles.
- Milestone 5 update: Diff/Comments panes now scroll independently with a focus toggle.
- Milestone 6 complete: Bitbucket Cloud publishing flow with PR comment composition.
- Milestone 7 complete: Keyboard help overlay, status bar, retry/error views, cancellation support, and structured logging.
- Milestone 8 complete: Improved Resilience and UI Focus (per-file errors, partial review success, retry in Comments tab, visual focus borders).

## What was implemented
- TUI entry point: `cmd/reviewer/main.go`
- Base Bubble Tea model with tabs: `internal/app/model.go`
- Config load/save with XDG support and env override: `internal/config/config.go`
- Env helpers for keys/tokens: `internal/config/env.go`
- Git helpers + tests: `internal/git/git.go`, `internal/git/git_test.go`
- Wizard flow: repo detection + branch/base picker persisted to config.
- Diff parser + types: `internal/git/diff.go` + tests in `internal/git/diff_test.go`
- Diff tab UI: file list + diff pane in `internal/app/model.go`
- Guideline scanning + hashing: `internal/review/guidelines.go`
- Guideline selection wizard (multi-select, add path, free-text input) + hash display: `internal/app/model.go`
- OpenRouter client + retries: `internal/llm/client.go`
- Review engine with prompt builders, per-file chunking, comment parsing/dedupe, verdict logic: `internal/review/engine.go`, `internal/review/prompts.go`, `internal/review/types.go`, `internal/review/diff_render.go`
- Comments + Verdict views now render basic review output and progress in `internal/app/model.go`
- Comments tab: table view, severity/file filters, detail pane, and publish include/exclude toggles in `internal/app/model.go`
- Added independent scrolling viewports for Comments detail pane and Diff pane focus switching via Tab in `internal/app/model.go`.
- Added model selection step to the wizard, improved review progress status (success/failure), warning for dropped comments, and LLM request logging to a cache file.
- Bitbucket integration: New `internal/bitbucket` package for API client and Markdown composer.
- Publish tab: TUI UI for Bitbucket workspace, repo, PR ID configuration and publishing execution.
- Persistence: Storing non-secret Bitbucket configuration to `config.json`.
- Dependencies: Bubble Tea, Bubbles, Lip Gloss added to `go.mod`/`go.sum`
- Milestone 7: Added `internal/logger` for structured JSON logging. Updated `cmd/reviewer/main.go` to support `--debug`, `--branch`, `--base`, `--model`, and `--guideline` flags. Added help overlay (?), status bar, and improved error views with centering. Added cancellation support for both review and publish processes via `context.Context`.

## How to run
- `go run ./cmd/reviewer`
- `go run ./cmd/reviewer --base main --branch my-feature --debug`

## Suggested next step
- Add `--no-tui` mode for CI/CD integration.
- Implement comment editing modal in Comments tab.

## Notes
- Config is stored under `~/.config/reviewer/config.json` unless `CODE_REVIEWER_CONFIG_DIR` is set.
- Bitbucket and OpenRouter tokens are read from env only (not persisted).
