# PLAN.md — LLM Code Reviewer TUI (Go + Bubble Tea, OpenRouter, Bitbucket Cloud publish)

## 0) Goal
Build an interactive TUI app that reviews **local git diffs** (branch vs base branch), generates structured review comments via **OpenRouter LLM**, lets the user triage/edit/exclude comments while viewing them **side-by-side with diff**, and optionally **publishes selected comments to Bitbucket Cloud PR** (using access token), including an overall **GO / NO-GO verdict**.

### In-scope (v1)
- Local git diff only: `branch` vs `baseBranch`
- Model selection (OpenRouter)
- Guideline selection:
  - Select local markdown guideline files (profiles)
  - Choose among multiple profiles
  - Free-text guideline input in TUI
- Interactive Bubble Tea TUI:
  - wizard → dashboard
  - progress indicators for network/LLM calls
  - tabs, tables, lists, pager, text inputs
- Review output:
  - comments with severity (NIT/SUGGESTION/ISSUE/BLOCKER), suggestion when needed, and a verdict (GO/NO-GO)
- Publish workflow:
  - publish to Bitbucket Cloud on user behalf (token from ENV or prompt at publish time)
  - allow excluding individual comments from publish

### Out-of-scope (v1)
- Auto-fix code generation
- Git commit creation
- PR creation
- Remote diff fetching (Bitbucket diff retrieval can be v2)

---

## 1) UX Overview

### 1.1 Wizard flow (first run per session)
1) **Repo context**
   - detect git repo (or allow user to select path)
2) **Branch selection**
   - select `branch` and `baseBranch` from local refs (with search)
3) **Model selection**
   - list models (preset list + free input)
4) **Guidelines**
   - choose one or multiple guideline profiles from disk
   - optional free-text guideline input (appended)
5) **Run review**
   - show progress bar + status messages

Then enter dashboard.

### 1.2 Dashboard tabs
Tabs (top nav, left/right arrows):
- **Diff**
  - file list panel + diff pager panel (split view)
- **Comments**
  - table/list of comments with filters + detail pane
- **Verdict**
  - summary, key blockers, GO/NO-GO decision rationale
- **Publish**
  - Bitbucket Cloud publish config + per-comment include/exclude + publish action
- **Config**
  - model, branches, guidelines, token/key status; re-run review

### 1.3 Key interactions
- `Tab` cycles focus between panes
- `/` opens search/filter in current list/table
- `Enter` opens detail or toggles selection depending on context
- `Space` toggles include/exclude for publish on a comment
- `e` edit comment text (optional v1: allow editing in a modal)
- `r` re-run review (keeps config)
- `p` publish (from Publish tab)
- `q` quit, `Esc` back/close modal

---

## 2) Data Model (Core Types)

### 2.1 Diff types
- `RepoInfo { RootPath string }`
- `DiffFile { Path string, Hunks []DiffHunk }`
- `DiffHunk { Header string, Lines []DiffLine, OldStart, OldLines, NewStart, NewLines int }`
- `DiffLine { Kind: Context|Add|Del, OldLine int?, NewLine int?, Text string }`

Also store raw unified diff text for LLM input.

### 2.2 Review types
- `Severity = NIT | SUGGESTION | ISSUE | BLOCKER`
- `ReviewComment {
    ID string (stable hash)
    FilePath string
    Range { StartLine int, EndLine int } // new-file line numbers preferred
    Severity Severity
    Title string
    Body string
    Suggestion *string // optional
    Evidence *string   // optional snippet
    Tags []string      // optional (security/perf/etc)
    Publish bool       // default true
  }`

- `Verdict {
    Decision GO|NO_GO
    Summary string
    Rationale []string
    Stats { Nit, Suggestion, Issue, Blocker int }
  }`

- `ReviewResult { Comments []ReviewComment, Verdict Verdict, Model string, GuidelineHash string, GeneratedAt time }`

### 2.3 Publish types (Bitbucket Cloud)
- `BitbucketConfig {
    Workspace string
    RepoSlug string
    PullRequestID int
    Token string (not persisted in plaintext; read env/prompt)
  }`

- `PublishPlan { Selected []ReviewComment, Skipped []ReviewComment }`
- `PublishResult { Posted int, Failed int, Errors []error }`

---

## 3) Configuration & Persistence

### 3.1 ENV behavior
- OpenRouter API key:
  - read `OPENROUTER_API_KEY`
  - if missing: prompt **before running review**
- Bitbucket token:
  - read `BITBUCKET_TOKEN` (or `BITBUCKET_ACCESS_TOKEN`)
  - if missing: prompt **only when publishing**
- Optional:
  - `OPENROUTER_BASE_URL` (default OpenRouter)
  - `CODE_REVIEWER_CONFIG_DIR` (optional override)

### 3.2 Local config file
Persist non-secret user preferences:
- last used branch/base
- last used model
- guideline selections (paths) + last free-text guideline
- UI preferences (wrap, theme)
Store under:
- `$XDG_CONFIG_HOME/reviewer/config.json` (or `~/.config/reviewer/config.json`)

Do NOT store tokens/keys unless user explicitly opts-in (v2).

---

## 4) Git Integration (Local only)

### 4.1 Branch listing
Use `go-git` or shell out to `git` (recommended for correctness/compat):
- `git rev-parse --show-toplevel`
- `git for-each-ref --format="%(refname:short)" refs/heads refs/remotes`

### 4.2 Diff generation (branch vs base)
Preferred: unified diff with enough context to comment:
- `git diff --unified=3 <baseBranch>...<branch>`
Store both:
- raw diff text
- parsed structure (files/hunks/lines)

### 4.3 Diff parsing
Implement a robust unified diff parser:
- detect `diff --git a/... b/...`
- capture file path `b/...`
- parse hunks `@@ -oldStart,oldLines +newStart,newLines @@`
- line prefixes: `+`, `-`, ` `

Map comment ranges to `new` line numbers.

---

## 5) OpenRouter LLM Integration

### 5.1 Model selection
- Provide a curated list (hardcoded JSON in repo) + free input
- Store last selection in config

### 5.2 Prompt strategy (structured output)
Use JSON output contract to avoid parsing ambiguity.

**System prompt**: strict reviewer persona; follow guidelines; be concise; cite diff evidence.
**User prompt** includes:
- selected guidelines (merged text)
- severity scale definition (NIT/SUGGESTION/ISSUE/BLOCKER)
- required schema
- diff (possibly chunked)

### 5.3 Output schema (LLM response)
Require a single JSON object:
```json
{
  "comments": [
    {
      "filePath": "path",
      "startLine": 10,
      "endLine": 10,
      "severity": "BLOCKER",
      "title": "Short title",
      "body": "Detailed comment",
      "suggestion": "Optional suggestion",
      "evidence": "Optional snippet"
    }
  ],
  "verdict": {
    "decision": "GO",
    "summary": "Short summary",
    "rationale": ["..."],
    "stats": { "nit": 1, "suggestion": 2, "issue": 1, "blocker": 0 }
  }
}
```

### 5.4 Chunking strategy (important for large diffs)

If diff exceeds token budget:

* chunk by file
* or chunk by hunks
* run LLM per chunk → merge comments
* final pass to compute verdict (or compute verdict from merged severity stats + a final summary prompt)

v1 recommended approach:

* Per-file review calls (parallel with concurrency limit)
* Then a final summarization call to produce verdict (hybrid logic)

### 5.5 Verdict hybrid logic

Rule-based default:

* any `BLOCKER` => `NO_GO`
  Else:
* LLM summary can still choose `NO_GO` if it detects systemic risk, but must justify in `rationale`
  Final decision:
* `NO_GO` if rule says NO_GO OR LLM says NO_GO (with rationale)
* Provide clear “why” in Verdict tab

### 5.6 Networking & reliability

* Timeouts + retries with backoff
* Show progress per file + total
* Cancellation (Ctrl+C) cancels in-flight requests

---

## 6) Bitbucket Cloud Publishing

### 6.1 Publish prerequisites (v1)

User provides:

* `workspace`
* `repoSlug`
* `pullRequestID`

These can be entered in Publish tab and optionally stored (non-secret).

### 6.2 Publishing strategy

Bitbucket Cloud supports:

* PR general comments
* Inline comments (anchored to file/line) depending on endpoint/payload

v1 plan (min-risk):

* Support **PR general comments** first (single comment containing multiple items)
* Optionally support inline comments if API payload is stable; implement behind a feature flag.

### 6.3 Publish format

Compose a markdown comment:

* Header: model + branch/base + verdict
* Section per comment:

  * severity badge
  * file:line range
  * title + body
  * suggestion (if any)
    Exclude comments toggled off.

### 6.4 Auth

* token from env or prompt
* store in memory only
* never log token

### 6.5 Failure modes

* token invalid → show error + retry prompt
* PR not found → show validation error
* partial failures → show result breakdown

---

## 7) Bubble Tea TUI Architecture

### 7.1 Packages layout (proposal)

* `/cmd/reviewer` main entry
* `/internal/app` bubbletea model + update/view
* `/internal/ui` components (tabs, lists, modals)
* `/internal/git` repo detection, branch list, diff fetch, diff parse
* `/internal/llm` openrouter client, prompts, chunking, schema validation
* `/internal/review` merge, scoring, verdict hybrid logic
* `/internal/bitbucket` publish client + markdown composer
* `/internal/config` load/save config, env helpers
* `/internal/util` logging, errors, concurrency, cancellation

### 7.2 UI components to use

* `bubbles/list` for branches/models/guidelines/files
* `bubbles/textinput` for:

  * free-text guideline
  * workspace/repo/pr id
  * API key prompt (masked)
* `bubbles/progress` for network progress
* `bubbles/table` for comments table
* `bubbles/paginator` for diff pager and comment detail pager
* Custom:

  * split pane layout (files list + diff pager)
  * modal dialogs (prompt for secrets, confirm publish, edit comment)

### 7.3 App state machine

States:

* `StateWizard`

  * steps: Repo → Branches → Model → Guidelines → Run
* `StateDashboard`

  * activeTab: Diff|Comments|Verdict|Publish|Config
* `StateModal`

  * SecretPrompt(OpenRouter/Bitbucket)
  * ConfirmPublish
  * EditComment
* `StateRunning`

  * LLM review in progress (can overlay on dashboard)

### 7.4 Concurrency model

* Use `context.Context` for cancellation
* LLM per file in goroutines with semaphore (e.g. 3-5)
* Each completion emits `tea.Msg` updating progress and results

---

## 8) Validation & Safety

* Validate branch/base exist
* Prevent reviewing empty diff (show message)
* Validate guideline files exist + readable
* Validate LLM output JSON:

  * strict schema validation (use `encoding/json` + manual checks)
  * if invalid: show parse error + optionally retry with “fix JSON” prompt (bounded retries)

---

## 9) Logging & Diagnostics

* Use structured logs to file under cache dir, but:

  * never log API keys/tokens
  * optionally log request IDs and timing
* Add `--debug` flag to show more UI diagnostics

---

## 10) CLI Flags (minimal)

Even though it’s TUI-first, add flags for automation:

* `--branch`, `--base`
* `--model`
* `--guideline path` (repeatable)
* `--no-tui` (optional later)
* `--version`

v1: flags override defaults, still launches TUI.

---

## 11) Milestones & Implementation Steps (AI-agent friendly)

### Milestone 1 — Project skeleton

* Init Go module
* Add Bubble Tea + Bubbles dependencies
* Implement base `Model` with tabs and placeholder views
* Add config load/save
* Add env key detection helpers

### Milestone 2 — Git diff pipeline

* Detect repo root
* List branches/base branches
* Generate unified diff (shell out to git)
* Parse diff into structures
* Diff tab: file list + diff pager

### Milestone 3 — Guidelines UX

* Add guideline file picker:

  * scan repo for `.review.md`, `.review/*.md`, or user-chosen paths
* Multi-select profiles
* Add free-text guideline input modal
* Compute `GuidelineHash` (for caching + display)

### Milestone 4 — OpenRouter client + review generation

* Implement OpenRouter client (chat/completions)
* Prompt builder + JSON schema contract
* Chunking strategy (per file)
* Progress UI while reviewing
* Merge comments + dedupe by stable hash
* Verdict: hybrid logic + final summary prompt

### Milestone 5 — Comments triage UX

* Comments tab:

  * table with columns: Sev | File | Line | Title | Publish?
  * filters: severity, file
  * detail pane shows body/suggestion/evidence
* Toggle publish include/exclude
* Optional edit comment modal (title/body/suggestion)

### Milestone 6 — Publish to Bitbucket Cloud

* Publish tab config inputs:

  * workspace, repoSlug, PR id
* Token prompt if missing
* Compose markdown and post PR comment
* Show results + errors
* Persist non-secret publish config

### Milestone 7 — Polish

* Keyboard help overlay
* Better status bar
* Robust error views + retry actions
* Cancellation support
* Small performance improvements + caching (optional)

---

## 12) Open Questions / Future Enhancements (v2+)

* Fetch Bitbucket PR metadata and diff automatically
* Inline comments anchoring for Bitbucket Cloud (file/line anchors)
* Multiple guideline profiles with weights
* Model presets fetched from OpenRouter API
* Local caching of LLM results by diff hash
* Export review as markdown or JSON
* Pre-commit / CI mode (non-interactive)

---

## 13) Acceptance Criteria (v1)

* User can select branch/base/model/guidelines in TUI
* App reads OpenRouter key from ENV or prompts
* App shows diff and generated comments side-by-side (via tabs/panes)
* Each comment includes severity and optional suggestion
* App produces GO/NO-GO verdict with rationale
* User can exclude comments from publish
* App can publish selected comments to Bitbucket Cloud PR using token from ENV or prompt
* Network operations display progress and are cancelable

