# Handover

## Status
- Milestone 1 complete: Go module initialized, Bubble Tea scaffolded, config load/save, env helpers.
- Milestone 2 in progress: git repo detection, branch listing, and diff generation helpers implemented (tests included).
- Wizard flow now detects repo and lets users select base/branch, persisting selections to config.
- Milestone 2 complete: diff parsing types and Diff tab (file list + diff pane) wired to generated diff.
- Milestone 3 complete: guideline scanning, multi-select, free-text input, and guideline hash display.

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
- Dependencies: Bubble Tea, Bubbles, Lip Gloss added to `go.mod`/`go.sum`

## How to run
- `go run ./cmd/reviewer`

## Suggested next step (Milestone 2)
- Start Milestone 3: guideline file scanning and multi-select picker in the wizard.

## Notes
- Config is stored under `~/.config/reviewer/config.json` unless `CODE_REVIEWER_CONFIG_DIR` is set.
- Bitbucket and OpenRouter tokens are read from env only (not persisted).
