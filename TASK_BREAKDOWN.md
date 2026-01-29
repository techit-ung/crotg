# TAKS_BREAKDOWN.md

## Milestone 1 — Project skeleton
- [x] Initialize Go module and add Bubble Tea/Bubbles dependencies
- [x] Implement base Bubble Tea model with tab scaffolding and placeholder views
- [x] Implement config load/save helpers
- [x] Add environment key detection utilities

## Milestone 2 — Git diff pipeline
- [x] Detect repo root path
- [x] List local branches and allow branch/base selection
- [x] Generate unified diff (shelling out to git) and store raw text
- [x] Parse diff into structured types (`DiffFile`, `DiffHunk`, `DiffLine`)
- [x] Build Diff tab UI (file list + diff pager)

## Milestone 3 — Guidelines UX
- [x] Scan repo for guideline profiles (`.review.md`, `.review/*.md`, custom paths)
- [x] Implement multi-select picker for guideline profiles
- [x] Add free-text guideline input modal and append logic
- [x] Compute and display `GuidelineHash`

## Milestone 4 — OpenRouter client + review generation
- [x] Implement OpenRouter API client (chat/completions) with progress UI
- [x] Build prompt builder + JSON schema contract for review output
- [x] Implement diff chunking strategy per file and invoke LLM
- [x] Merge generated comments, dedupe via stable hash, and build verdict

## Milestone 5 — Comments triage UX
- [x] Implement Comments tab table with filters (severity, file)
- [x] Add detail pane showing body, suggestion, evidence
- [x] Allow toggling publish include/exclude per comment
- [ ] Optional: add comment edit modal for title/body/suggestion

## Milestone 6 — Publish to Bitbucket Cloud
- [ ] Build Publish tab inputs (workspace, repo slug, PR id, token status)
- [ ] Prompt for Bitbucket token when missing at publish time
- [ ] Compose and post markdown comments to Bitbucket Cloud PR
- [ ] Show publish results/errors and persist non-secret config

## Milestone 7 — Polish & Validation
- [ ] Implement keyboard help overlay, status bar, and retry/error views
- [ ] Add cancellation support and progress indicators for network ops
- [ ] Validate branches, empty diff, guideline files, and LLM JSON schema
- [ ] Implement structured logging with optional `--debug` flag

## CLI & Future Enhancements
- [ ] Wire CLI flags (`--branch`, `--base`, `--model`, `--guideline`, `--no-tui`, `--version`)
- [ ] Track open questions/future enhancements for v2 (Bitbucket diff fetch, caching, exports, etc.)
