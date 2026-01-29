# Handover

## Status
- Milestone 1 complete: Go module initialized, Bubble Tea scaffolded, config load/save, env helpers.
- Milestone 2 complete: repo detection, branch selection, diff generation/parsing, Diff tab UI.
- Milestone 3 complete: guideline scanning, multi-select, free-text input, and guideline hash display.
- Milestone 4 complete: OpenRouter client, prompt builders, per-file review chunking, comment dedupe, verdict generation.
- Milestone 5 complete: comments table with filters, detail pane, and publish toggles.

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
- Dependencies: Bubble Tea, Bubbles, Lip Gloss added to `go.mod`/`go.sum`

## How to run
- `go run ./cmd/reviewer`

## Suggested next step (Milestone 6)
- Build Publish tab config inputs and Bitbucket Cloud publish flow.

## Notes
- Config is stored under `~/.config/reviewer/config.json` unless `CODE_REVIEWER_CONFIG_DIR` is set.
- Bitbucket and OpenRouter tokens are read from env only (not persisted).
