package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/bitbucket"
	"github.com/techitung-arunyawee/code-reviewer-2/internal/config"
	"github.com/techitung-arunyawee/code-reviewer-2/internal/git"
	"github.com/techitung-arunyawee/code-reviewer-2/internal/llm"
	"github.com/techitung-arunyawee/code-reviewer-2/internal/review"
)

type Model struct {
	tabs   []string
	active int
	width  int
	height int

	inWizard   bool
	wizardStep wizardStep
	repoRoot   string
	branches   []string
	cursor     int
	baseBranch string
	branch     string
	err        error
	cfg        config.Config

	diffText  string
	diffFiles []git.DiffFile
	diffErr   error
	diffFile  int
	diffView  viewport.Model

	guidelineOptions  []string
	guidelineSelected map[string]bool
	guidelineCursor   int
	guidelineErr      error
	guidelineHash     string
	pathInput         textinput.Model
	freeTextInput     textinput.Model
	keyInput          textinput.Model
	modelInput        textinput.Model
	openRouterKey     string
	branchFilterInput textinput.Model
	modelOptions      []string
	modelCursor       int

	reviewRunning  bool
	reviewErr      error
	reviewResult   review.Result
	reviewProgress reviewProgressMsg
	reviewUpdates  <-chan tea.Msg

	commentsTable          table.Model
	commentsIndexMap       []int
	commentsFileFilter     textinput.Model
	commentsFilterActive   bool
	commentsSeverityFilter review.Severity
	commentsTableWidth     int
	commentsTableHeight    int
	commentsDetailView     viewport.Model
	commentsPanelFocus     panelFocus
	diffPanelFocus         panelFocus

	publishWorkspaceInput textinput.Model
	publishRepoSlugInput  textinput.Model
	publishPRIDInput      textinput.Model
	publishTokenInput     textinput.Model
	publishToken          string
	publishRunning        bool
	publishError          error
	publishResultID       string

	showHelp bool
	cancel   context.CancelFunc

	initialBase      string
	initialBranch    string
	initialModel     string
	initialGuideline string
}

func NewModel(base, branch, model, guideline string) Model {
	pathInput := textinput.New()
	pathInput.Placeholder = "path/to/guideline.md"
	freeTextInput := textinput.New()
	freeTextInput.Placeholder = "Free-text guideline (optional)"
	keyInput := textinput.New()
	keyInput.Placeholder = "OpenRouter API Key"
	keyInput.EchoMode = textinput.EchoPassword
	keyInput.EchoCharacter = '*'
	branchFilterInput := textinput.New()
	branchFilterInput.Placeholder = "Filter branches"
	modelInput := textinput.New()
	modelInput.Placeholder = "Model (e.g. openai/gpt-4o-mini)"
	commentsFileFilter := textinput.New()
	commentsFileFilter.Placeholder = "Filter by file path"

	publishWorkspaceInput := textinput.New()
	publishWorkspaceInput.Placeholder = "Bitbucket Workspace (e.g. acme)"
	publishRepoSlugInput := textinput.New()
	publishRepoSlugInput.Placeholder = "Repo Slug (e.g. my-repo)"
	publishPRIDInput := textinput.New()
	publishPRIDInput.Placeholder = "PR ID (e.g. 123)"
	publishTokenInput := textinput.New()
	publishTokenInput.Placeholder = "Bitbucket App Password or Token"
	publishTokenInput.EchoMode = textinput.EchoPassword
	publishTokenInput.EchoCharacter = '*'

	diffView := viewport.New(0, 0)
	commentsDetailView := viewport.New(0, 0)
	commentsTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "Sev", Width: 9},
			{Title: "File", Width: 24},
			{Title: "Line", Width: 8},
			{Title: "Title", Width: 30},
			{Title: "Pub", Width: 5},
		}),
		table.WithFocused(true),
	)

	return Model{
		tabs: []string{
			"Diff",
			"Comments",
			"Verdict",
			"Publish",
			"Config",
		},
		inWizard:           true,
		wizardStep:         wizardRepo,
		pathInput:          pathInput,
		freeTextInput:      freeTextInput,
		keyInput:           keyInput,
		branchFilterInput:  branchFilterInput,
		modelInput:         modelInput,
		diffView:           diffView,
		diffPanelFocus:     panelFocusLeft,
		commentsFileFilter: commentsFileFilter,
		commentsTable:      commentsTable,
		commentsDetailView: commentsDetailView,
		commentsPanelFocus: panelFocusLeft,
		publishWorkspaceInput: publishWorkspaceInput,
		publishRepoSlugInput:  publishRepoSlugInput,
		publishPRIDInput:      publishPRIDInput,
		publishTokenInput:     publishTokenInput,
		initialBase:      base,
		initialBranch:    branch,
		initialModel:     model,
		initialGuideline: guideline,
		modelOptions: []string{
			review.DefaultModel,
			"Custom...",
		},
	}
}

func (m Model) Init() tea.Cmd {
	slog.Info("Starting code-reviewer-2")
	return tea.Batch(loadConfigCmd(), detectRepoCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case configLoadedMsg:
		m.cfg = msg.cfg
		if m.initialBase != "" {
			m.cfg.LastBase = m.initialBase
		}
		if m.initialBranch != "" {
			m.cfg.LastBranch = m.initialBranch
		}
		if m.initialModel != "" {
			m.cfg.LastModel = m.initialModel
		}
		m.publishWorkspaceInput.SetValue(msg.cfg.PublishWorkspace)
		m.publishRepoSlugInput.SetValue(msg.cfg.PublishRepoSlug)
		if msg.cfg.PublishPRID != 0 {
			m.publishPRIDInput.SetValue(fmt.Sprintf("%d", msg.cfg.PublishPRID))
		}
		return m, nil
	case configSavedMsg:
		return m, nil
	case diffLoadedMsg:
		m.diffText = msg.raw
		m.diffFiles = msg.files
		m.diffErr = msg.err
		if msg.err == nil {
			m.diffFile = 0
			m.updateDiffViewportContent()
			m.updateDiffViewportLayout()
			return m, m.maybeStartReview()
		}
		return m, nil
	case guidelinesScannedMsg:
		m.guidelineOptions = msg.paths
		m.guidelineErr = msg.err
		m.guidelineSelected = make(map[string]bool)

		selectedGuidelines := m.cfg.Guidelines
		if m.initialGuideline != "" {
			resolved, err := review.ResolveGuidelinePath(m.repoRoot, m.initialGuideline)
			if err == nil {
				selectedGuidelines = []string{resolved}
				// Ensure the specified guideline is in the options even if not found by scan
				found := false
				for _, p := range m.guidelineOptions {
					if p == resolved {
						found = true
						break
					}
				}
				if !found {
					m.guidelineOptions = append(m.guidelineOptions, resolved)
					sort.Strings(m.guidelineOptions)
				}
			}
		}

		for _, path := range msg.paths {
			for _, selected := range selectedGuidelines {
				if path == selected {
					m.guidelineSelected[path] = true
					break
				}
			}
		}
		return m, hashGuidelinesCmd(m.selectedGuidelines(), m.cfg.FreeGuideline)
	case guidelineHashMsg:
		if msg.err != nil {
			m.guidelineErr = msg.err
			return m, nil
		}
		m.guidelineHash = msg.hash
		return m, nil
	case reviewStartedMsg:
		m.reviewRunning = true
		m.reviewErr = nil
		m.reviewUpdates = msg.updates
		m.reviewProgress = reviewProgressMsg{}
		m.cancel = msg.cancel
		return m, listenReviewCmd(msg.updates)
	case reviewProgressMsg:
		m.reviewProgress = msg
		if m.reviewUpdates != nil {
			return m, listenReviewCmd(m.reviewUpdates)
		}
		return m, nil
	case reviewCompletedMsg:
		m.reviewRunning = false
		m.reviewUpdates = nil
		m.reviewErr = msg.err
		if msg.err != nil {
			slog.Error("Review failed", "error", msg.err)
		} else {
			slog.Info("Review completed", "comments", len(msg.result.Comments))
			m.reviewResult = msg.result
			m.refreshCommentsTable()
			m.updateCommentsTableLayout()
		}
		return m, nil
	case publishStartedMsg:
		m.publishRunning = true
		m.publishError = nil
		m.publishResultID = ""
		m.cancel = msg.cancel
		return m, nil
	case publishCompletedMsg:
		m.publishRunning = false
		m.publishError = msg.err
		m.publishResultID = msg.resultID
		if msg.err != nil {
			slog.Error("Publish failed", "error", msg.err)
		} else {
			slog.Info("Publish successful", "id", msg.resultID)
			// Update config with non-secret publish settings
			m.cfg.PublishWorkspace = m.publishWorkspaceInput.Value()
			m.cfg.PublishRepoSlug = m.publishRepoSlugInput.Value()
			var prID int
			fmt.Sscanf(m.publishPRIDInput.Value(), "%d", &prID)
			m.cfg.PublishPRID = prID
			return m, saveConfigCmd(m.cfg)
		}
		return m, nil
	case repoDetectedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.repoRoot = msg.root
		m.branches = msg.branches
		m.err = nil
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateDiffViewportLayout()
		m.updateCommentsTableLayout()
		return m, nil
	case tea.KeyMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.inWizard {
			return m.updateWizard(msg)
		}
		if m.tabs[m.active] == "Diff" {
			return m.updateDiffTab(msg)
		}
		if m.tabs[m.active] == "Comments" {
			return m.updateCommentsTab(msg)
		}
		if m.tabs[m.active] == "Publish" {
			return m.updatePublishTab(msg)
		}
		if m.tabs[m.active] == "Config" {
			return m.updateConfigTab(msg)
		}
		slog.Debug("Key pressed", "key", msg.String(), "tab", m.tabs[m.active])
		switch msg.String() {
		case "ctrl+c":
			if (m.reviewRunning || m.publishRunning) && m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "q":
			if !m.commentsFilterActive {
				return m, tea.Quit
			}
		case "esc":
			if (m.reviewRunning || m.publishRunning) && m.cancel != nil {
				m.cancel()
				m.reviewRunning = false
				m.publishRunning = false
				return m, nil
			}
		case "right", "l":
			m.active = (m.active + 1) % len(m.tabs)
			return m, nil
		case "left", "h":
			m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
			return m, nil
		case "?":
			m.showHelp = true
			return m, nil
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	var content string
	if m.inWizard {
		content = m.renderWizard()
	} else {
		tabLine := m.renderTabs()
		mainContent := m.renderActiveView()
		content = lipgloss.JoinVertical(lipgloss.Top, tabLine, mainContent)
	}

	// Ensure content takes up all space except status bar
	content = lipgloss.NewStyle().Height(m.height - 1).MaxHeight(m.height - 1).Render(content)
	statusBar := m.renderStatusBar()
	view := lipgloss.JoinVertical(lipgloss.Top, content, statusBar)

	if m.showHelp {
		return m.renderHelpOverlay(view)
	}
	return view
}

func (m Model) renderTabs() string {
	activeStyle := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)

	rendered := make([]string, 0, len(m.tabs))
	for i, tab := range m.tabs {
		if i == m.active {
			rendered = append(rendered, activeStyle.Render(tab))
			continue
		}
		rendered = append(rendered, inactiveStyle.Render(tab))
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, rendered...)
}

type wizardStep int

const (
	wizardRepo wizardStep = iota
	wizardBaseBranch
	wizardBranch
	wizardModel
	wizardModelInput
	wizardGuidelines
	wizardGuidelinePath
	wizardFreeGuideline
	wizardOpenRouterKey
)

type panelFocus int

const (
	panelFocusLeft panelFocus = iota
	panelFocusRight
)

type configLoadedMsg struct {
	cfg config.Config
	err error
}

type repoDetectedMsg struct {
	root     string
	branches []string
	err      error
}

type configSavedMsg struct {
	err error
}

type diffLoadedMsg struct {
	raw   string
	files []git.DiffFile
	err   error
}

type guidelinesScannedMsg struct {
	paths []string
	err   error
}

type guidelineHashMsg struct {
	hash string
	err  error
}

type reviewStartedMsg struct {
	updates <-chan tea.Msg
	cancel  context.CancelFunc
}

type reviewProgressMsg struct {
	completed int
	total     int
	failed    int
	file      string
	lastError string
}

type reviewCompletedMsg struct {
	result review.Result
	err    error
}

type publishStartedMsg struct {
	cancel context.CancelFunc
}

type publishCompletedMsg struct {
	resultID string
	err      error
}

func loadConfigCmd() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		return configLoadedMsg{cfg: cfg, err: err}
	}
}

func detectRepoCmd() tea.Cmd {
	return func() tea.Msg {
		cwd, err := os.Getwd()
		if err != nil {
			return repoDetectedMsg{err: err}
		}

		repoInfo, err := git.DetectRepoRoot(cwd)
		if err != nil {
			return repoDetectedMsg{err: err}
		}

		branches, err := git.ListBranches(repoInfo.RootPath)
		if err != nil {
			return repoDetectedMsg{err: err}
		}

		sort.Strings(branches)
		return repoDetectedMsg{root: repoInfo.RootPath, branches: branches}
	}
}

func saveConfigCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		return configSavedMsg{err: config.Save(cfg)}
	}
}

func generateDiffCmd(repoRoot, baseBranch, branch string) tea.Cmd {
	return func() tea.Msg {
		diff, err := git.GenerateDiff(repoRoot, baseBranch, branch)
		if err != nil {
			return diffLoadedMsg{err: err}
		}

		files, err := git.ParseUnifiedDiff(diff)
		if err != nil {
			return diffLoadedMsg{raw: diff, err: err}
		}

		return diffLoadedMsg{raw: diff, files: files}
	}
}

func scanGuidelinesCmd(repoRoot string, extra []string) tea.Cmd {
	return func() tea.Msg {
		paths, err := review.ScanGuidelineFiles(repoRoot, extra)
		return guidelinesScannedMsg{paths: paths, err: err}
	}
}

func hashGuidelinesCmd(paths []string, freeText string) tea.Cmd {
	return func() tea.Msg {
		hash, err := review.HashGuidelines(paths, freeText)
		return guidelineHashMsg{hash: hash, err: err}
	}
}

func (m Model) updateWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}

	if m.err != nil {
		if msg.String() == "r" {
			m.err = nil
			return m, detectRepoCmd()
		}
		return m, nil
	}

	switch m.wizardStep {
	case wizardRepo:
		if msg.String() == "enter" {
			m.wizardStep = wizardBaseBranch
			m.cursor = m.initialBranchIndex(m.cfg.LastBase)
			m.branchFilterInput.SetValue("")
			m.branchFilterInput.SetCursor(0)
			m.branchFilterInput.Focus()
		}
	case wizardBaseBranch:
		switch msg.String() {
		case "up", "k":
			m.cursor = clamp(m.cursor-1, 0, len(m.filteredBranches())-1)
		case "down", "j":
			m.cursor = clamp(m.cursor+1, 0, len(m.filteredBranches())-1)
		case "enter":
			filtered := m.filteredBranches()
			if len(filtered) == 0 {
				return m, nil
			}
			m.baseBranch = filtered[m.cursor]
			m.wizardStep = wizardBranch
			m.cursor = m.initialBranchIndex(m.cfg.LastBranch)
			m.branchFilterInput.SetValue("")
			m.branchFilterInput.SetCursor(0)
			m.branchFilterInput.Focus()
			return m, nil
		default:
			var cmd tea.Cmd
			m.branchFilterInput, cmd = m.branchFilterInput.Update(msg)
			m.cursor = 0
			return m, cmd
		}
	case wizardBranch:
		switch msg.String() {
		case "up", "k":
			m.cursor = clamp(m.cursor-1, 0, len(m.filteredBranches())-1)
		case "down", "j":
			m.cursor = clamp(m.cursor+1, 0, len(m.filteredBranches())-1)
		case "b":
			m.wizardStep = wizardBaseBranch
			m.cursor = m.initialBranchIndex(m.baseBranch)
			m.branchFilterInput.SetValue("")
			m.branchFilterInput.SetCursor(0)
			m.branchFilterInput.Focus()
		case "enter":
			filtered := m.filteredBranches()
			if len(filtered) == 0 {
				return m, nil
			}
			m.branch = filtered[m.cursor]
			m.wizardStep = wizardModel
			m.modelCursor = m.initialModelIndex(m.cfg.LastModel)
			m.branchFilterInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.branchFilterInput, cmd = m.branchFilterInput.Update(msg)
			m.cursor = 0
			return m, cmd
		}
	case wizardModel:
		switch msg.String() {
		case "up", "k":
			m.modelCursor = clamp(m.modelCursor-1, 0, len(m.modelOptions)-1)
		case "down", "j":
			m.modelCursor = clamp(m.modelCursor+1, 0, len(m.modelOptions)-1)
		case "b":
			m.wizardStep = wizardModel
			m.modelCursor = m.initialModelIndex(m.cfg.LastModel)
			m.branchFilterInput.SetValue("")
			m.branchFilterInput.SetCursor(0)
			m.branchFilterInput.Focus()
		case "enter":
			if len(m.modelOptions) == 0 {
				return m, nil
			}
			selected := m.modelOptions[m.modelCursor]
			if selected == "Custom..." {
				m.wizardStep = wizardModelInput
				m.modelInput.SetValue(m.cfg.LastModel)
				m.modelInput.Focus()
				return m, nil
			}
			m.cfg.LastModel = selected
			m.wizardStep = wizardGuidelines
			m.guidelineCursor = 0
			m.guidelineErr = nil
			return m, scanGuidelinesCmd(m.repoRoot, m.cfg.Guidelines)
		}
	case wizardModelInput:
		switch msg.String() {
		case "esc":
			m.modelInput.Blur()
			m.wizardStep = wizardModel
			return m, nil
		case "b":
			m.modelInput.Blur()
			m.wizardStep = wizardModel
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.modelInput.Value())
			if value == "" {
				return m, nil
			}
			m.cfg.LastModel = value
			m.modelInput.Blur()
			m.wizardStep = wizardGuidelines
			m.guidelineCursor = 0
			m.guidelineErr = nil
			return m, scanGuidelinesCmd(m.repoRoot, m.cfg.Guidelines)
		default:
			var cmd tea.Cmd
			m.modelInput, cmd = m.modelInput.Update(msg)
			return m, cmd
		}
	case wizardGuidelines:
		switch msg.String() {
		case "up", "k":
			m.guidelineCursor = clamp(m.guidelineCursor-1, 0, len(m.guidelineOptions)-1)
		case "down", "j":
			m.guidelineCursor = clamp(m.guidelineCursor+1, 0, len(m.guidelineOptions)-1)
		case " ":
			if len(m.guidelineOptions) == 0 {
				return m, nil
			}
			path := m.guidelineOptions[m.guidelineCursor]
			m.guidelineSelected[path] = !m.guidelineSelected[path]
			return m, hashGuidelinesCmd(m.selectedGuidelines(), m.cfg.FreeGuideline)
		case "a":
			m.wizardStep = wizardGuidelinePath
			m.pathInput.Reset()
			m.pathInput.Focus()
			return m, nil
		case "b":
			m.wizardStep = wizardBranch
			m.cursor = m.initialBranchIndex(m.branch)
		case "enter":
			m.cfg.Guidelines = m.selectedGuidelines()
			m.wizardStep = wizardFreeGuideline
			m.freeTextInput.SetValue(m.cfg.FreeGuideline)
			m.freeTextInput.Focus()
			return m, nil
		}
	case wizardGuidelinePath:
		switch msg.String() {
		case "esc":
			m.wizardStep = wizardGuidelines
			return m, nil
		case "enter":
			resolved, err := review.ResolveGuidelinePath(m.repoRoot, m.pathInput.Value())
			if err != nil {
				m.guidelineErr = err
				return m, nil
			}
			if err := validateGuidelinePath(resolved); err != nil {
				m.guidelineErr = err
				return m, nil
			}
			if !m.hasGuidelineOption(resolved) {
				m.guidelineOptions = append(m.guidelineOptions, resolved)
				sort.Strings(m.guidelineOptions)
			}
			if m.guidelineSelected == nil {
				m.guidelineSelected = make(map[string]bool)
			}
			m.guidelineSelected[resolved] = true
			m.guidelineErr = nil
			m.wizardStep = wizardGuidelines
			return m, hashGuidelinesCmd(m.selectedGuidelines(), m.cfg.FreeGuideline)
		default:
			var cmd tea.Cmd
			m.pathInput, cmd = m.pathInput.Update(msg)
			return m, cmd
		}
	case wizardFreeGuideline:
		switch msg.String() {
		case "esc":
			m.freeTextInput.SetValue("")
			m.cfg.FreeGuideline = ""
			m.wizardStep = wizardGuidelines
			return m, nil
		case "b":
			m.wizardStep = wizardGuidelines
			return m, nil
		case "enter":
			m.cfg.FreeGuideline = strings.TrimSpace(m.freeTextInput.Value())
			m.cfg.LastBase = m.baseBranch
			m.cfg.LastBranch = m.branch
			if strings.TrimSpace(config.OpenRouterAPIKey()) == "" && strings.TrimSpace(m.openRouterKey) == "" {
				m.wizardStep = wizardOpenRouterKey
				m.keyInput.Reset()
				m.keyInput.Focus()
				return m, nil
			}
			m.inWizard = false
			return m, tea.Batch(
				saveConfigCmd(m.cfg),
				hashGuidelinesCmd(m.cfg.Guidelines, m.cfg.FreeGuideline),
				generateDiffCmd(m.repoRoot, m.baseBranch, m.branch),
			)
		default:
			var cmd tea.Cmd
			m.freeTextInput, cmd = m.freeTextInput.Update(msg)
			return m, cmd
		}
	case wizardOpenRouterKey:
		switch msg.String() {
		case "esc":
			m.keyInput.SetValue("")
			m.wizardStep = wizardFreeGuideline
			return m, nil
		case "b":
			m.wizardStep = wizardFreeGuideline
			return m, nil
		case "enter":
			m.openRouterKey = strings.TrimSpace(m.keyInput.Value())
			if m.openRouterKey == "" {
				return m, nil
			}
			m.inWizard = false
			return m, tea.Batch(
				saveConfigCmd(m.cfg),
				hashGuidelinesCmd(m.cfg.Guidelines, m.cfg.FreeGuideline),
				generateDiffCmd(m.repoRoot, m.baseBranch, m.branch),
			)
		default:
			var cmd tea.Cmd
			m.keyInput, cmd = m.keyInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) renderWizard() string {
	if m.err != nil {
		return m.renderErrorView(m.err, "Press r to retry, q to quit.")
	}
	header := lipgloss.NewStyle().Bold(true).Render("Setup Wizard")

	switch m.wizardStep {
	case wizardRepo:
		repoLine := "Detecting repository..."
		if m.repoRoot != "" {
			repoLine = fmt.Sprintf("Repository: %s", m.repoRoot)
		}
		return lipgloss.JoinVertical(
			lipgloss.Top,
			header,
			repoLine,
			"",
			"Press Enter to continue.",
		)
	case wizardBaseBranch:
		return m.renderBranchPicker("Select base branch", m.baseBranch)
	case wizardBranch:
		return m.renderBranchPicker("Select review branch", m.branch)
	case wizardModel:
		return m.renderModelPicker()
	case wizardModelInput:
		return m.renderModelInput()
	case wizardGuidelines:
		return m.renderGuidelinePicker()
	case wizardGuidelinePath:
		return m.renderGuidelinePathInput()
	case wizardFreeGuideline:
		return m.renderFreeGuidelineInput()
	case wizardOpenRouterKey:
		return m.renderOpenRouterKeyInput()
	default:
		return "loading..."
	}
}

func (m Model) renderActiveView() string {
	if m.reviewErr != nil && m.tabs[m.active] != "Config" {
		return m.renderErrorView(m.reviewErr, "Press r (in Config tab) to re-run review.")
	}

	switch m.tabs[m.active] {
	case "Diff":
		return m.renderDiffView()
	case "Comments":
		return m.renderCommentsView()
	case "Verdict":
		return m.renderVerdictView()
	case "Publish":
		return m.renderPublishView()
	case "Config":
		return m.renderConfigView()
	default:
		return fmt.Sprintf("%s view\n\nComing soon.", m.tabs[m.active])
	}
}

func (m *Model) updateConfigTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "right", "l":
		m.active = (m.active + 1) % len(m.tabs)
		return m, nil
	case "left", "h":
		m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
		return m, nil
	case "r":
		m.reviewResult = review.Result{}
		m.reviewRunning = true
		return m, m.maybeStartReview()
	}
	return m, nil
}

func (m Model) renderPublishView() string {
	if m.reviewRunning {
		return "\n  Review in progress, please wait..."
	}
	if m.reviewResult.GeneratedAt.IsZero() {
		return "\n  No review results to publish. Please run a review first."
	}

	header := lipgloss.NewStyle().Bold(true).Padding(1, 0).Render("Publish to Bitbucket Cloud")

	var statusLine string
	if m.publishRunning {
		statusLine = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("Publishing...")
	} else if m.publishError != nil {
		statusLine = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("Error: %v", m.publishError))
	} else if m.publishResultID != "" {
		statusLine = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(fmt.Sprintf("Success! Comment ID: %s", m.publishResultID))
	}

	// Calculate counts
	total := len(m.reviewResult.Comments)
	selected := 0
	for _, c := range m.reviewResult.Comments {
		if c.Publish {
			selected++
		}
	}

	summary := fmt.Sprintf("Summary: %d comments total, %d selected for publishing.", total, selected)
	if selected == 0 {
		summary += lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(" (Nothing will be published)")
	}

	form := lipgloss.JoinVertical(lipgloss.Left,
		"Workspace:", m.publishWorkspaceInput.View(),
		"Repo Slug:", m.publishRepoSlugInput.View(),
		"PR ID:    ", m.publishPRIDInput.View(),
		"Token:    ", m.publishTokenInput.View(),
	)

	hint := "Tab to cycle, Enter to confirm input, p to Publish to Bitbucket."
	if m.publishRunning {
		hint = "Publishing..."
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		summary,
		"",
		form,
		"",
		statusLine,
		"",
		hint,
	)
}

func (m Model) renderDiffView() string {
	if m.diffErr != nil {
		return m.renderErrorView(m.diffErr, "Check your branches and try again.")
	}
	if len(m.diffFiles) == 0 {
		return m.renderErrorView(errors.New("no changes detected"), "Make sure you selected the correct branches and have committed your changes.")
	}

	leftWidth, rightWidth := m.diffPaneWidths()
	height := m.diffPaneHeight()
	m.diffView.Width = rightWidth
	m.diffView.Height = height

	focusedStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	unfocusedStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("241"))

	leftPaneStyle := unfocusedStyle
	rightPaneStyle := unfocusedStyle

	if m.diffPanelFocus == panelFocusLeft {
		leftPaneStyle = focusedStyle
	} else {
		rightPaneStyle = focusedStyle
	}

	fileList := m.renderFileList(height - 2)
	diffPane := m.diffView.View()

	return lipgloss.JoinHorizontal(lipgloss.Top,
		leftPaneStyle.Width(leftWidth).Render(fileList),
		rightPaneStyle.Width(rightWidth).Render(diffPane),
	)
}

func (m Model) renderFileList(height int) string {
	visibleCount := height
	if visibleCount < 5 {
		visibleCount = 5
	}
	start, end := clampWindow(m.diffFile, len(m.diffFiles), visibleCount)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		file := m.diffFiles[i]
		cursor := "  "
		if i == m.diffFile {
			cursor = "> "
		}
		lines = append(lines, cursor+file.Path)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderFileDiff() string {
	if m.diffFile < 0 || m.diffFile >= len(m.diffFiles) {
		return ""
	}

	file := m.diffFiles[m.diffFile]
	lines := make([]string, 0)
	for _, hunk := range file.Hunks {
		lines = append(lines, hunk.Header)
		for _, line := range hunk.Lines {
			lines = append(lines, formatDiffLine(line))
		}
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func formatDiffLine(line git.DiffLine) string {
	switch line.Kind {
	case git.DiffLineAdd:
		return "+" + line.Text
	case git.DiffLineDel:
		return "-" + line.Text
	default:
		return " " + line.Text
	}
}

func (m Model) renderBranchPicker(title, selected string) string {
	header := lipgloss.NewStyle().Bold(true).Render(title)
	if len(m.branches) == 0 {
		return lipgloss.JoinVertical(lipgloss.Top, header, "No branches found.")
	}

	filtered := m.filteredBranches()
	if len(filtered) == 0 {
		return lipgloss.JoinVertical(
			lipgloss.Top,
			header,
			"Filter: "+m.branchFilterInput.View(),
			"",
			"No branches match the filter.",
		)
	}

	visibleCount := m.branchVisibleCount()
	start, end := clampWindow(m.cursor, len(filtered), visibleCount)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		branch := filtered[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		label := branch
		if branch == selected {
			label = fmt.Sprintf("%s (current)", branch)
		}
		lines = append(lines, cursor+label)
	}

	hint := "Type to filter, ↑/↓ to move, Enter to select."
	if m.wizardStep == wizardBranch {
		hint = "Type to filter, ↑/↓ to move, Enter to select, b to go back."
	}
	status := fmt.Sprintf("Showing %d-%d of %d (filtered from %d)", start+1, end, len(filtered), len(m.branches))
	return lipgloss.JoinVertical(lipgloss.Top, header, "Filter: "+m.branchFilterInput.View(), "", strings.Join(lines, "\n"), "", status, "", hint)
}

func (m Model) initialBranchIndex(branch string) int {
	if branch == "" {
		return 0
	}
	filtered := m.filteredBranches()
	for i, name := range filtered {
		if name == branch {
			return i
		}
	}
	return 0
}

func (m Model) initialModelIndex(model string) int {
	if model == "" {
		return 0
	}
	for i, option := range m.modelOptions {
		if option == model {
			return i
		}
	}
	return len(m.modelOptions) - 1
}

func (m Model) renderModelPicker() string {
	header := lipgloss.NewStyle().Bold(true).Render("Select model")
	if len(m.modelOptions) == 0 {
		return lipgloss.JoinVertical(lipgloss.Top, header, "No models configured.")
	}
	lines := make([]string, 0, len(m.modelOptions))
	for i, option := range m.modelOptions {
		cursor := "  "
		if i == m.modelCursor {
			cursor = "> "
		}
		label := option
		if option == m.cfg.LastModel {
			label = fmt.Sprintf("%s (current)", option)
		}
		lines = append(lines, cursor+label)
	}
	hint := "Use ↑/↓, Enter to select, b to go back."
	return lipgloss.JoinVertical(lipgloss.Top, header, strings.Join(lines, "\n"), "", hint)
}

func (m Model) renderModelInput() string {
	header := lipgloss.NewStyle().Bold(true).Render("Custom model")
	body := m.modelInput.View()
	hint := "Enter to continue, b to go back."
	return lipgloss.JoinVertical(lipgloss.Top, header, body, "", hint)
}

func (m Model) renderGuidelinePicker() string {
	header := lipgloss.NewStyle().Bold(true).Render("Select guideline profiles")
	if m.guidelineErr != nil {
		return lipgloss.JoinVertical(lipgloss.Top, header, fmt.Sprintf("Error: %s", m.guidelineErr))
	}

	if len(m.guidelineOptions) == 0 {
		return lipgloss.JoinVertical(
			lipgloss.Top,
			header,
			"No guideline files found.",
			"",
			"Press a to add a path or Enter to continue.",
		)
	}

	lines := make([]string, 0, len(m.guidelineOptions))
	for i, path := range m.guidelineOptions {
		cursor := "  "
		if i == m.guidelineCursor {
			cursor = "> "
		}
		checked := "[ ]"
		if m.guidelineSelected[path] {
			checked = "[x]"
		}
		label := m.formatGuidelineLabel(path)
		lines = append(lines, fmt.Sprintf("%s%s %s", cursor, checked, label))
	}

	hashLine := ""
	if m.guidelineHash != "" {
		hashLine = fmt.Sprintf("Guideline hash: %s", m.guidelineHash)
	}

	hint := "Use ↑/↓, Space to toggle, a to add path, Enter to continue, b to go back."
	return lipgloss.JoinVertical(lipgloss.Top, header, strings.Join(lines, "\n"), "", hashLine, "", hint)
}

func (m Model) renderGuidelinePathInput() string {
	header := lipgloss.NewStyle().Bold(true).Render("Add guideline path")
	body := m.pathInput.View()
	hint := "Enter to add, Esc to cancel."
	return lipgloss.JoinVertical(lipgloss.Top, header, body, "", hint)
}

func (m Model) renderFreeGuidelineInput() string {
	header := lipgloss.NewStyle().Bold(true).Render("Free-text guideline (optional)")
	body := m.freeTextInput.View()
	hint := "Enter to continue, b to go back."
	return lipgloss.JoinVertical(lipgloss.Top, header, body, "", hint)
}

func (m Model) renderOpenRouterKeyInput() string {
	header := lipgloss.NewStyle().Bold(true).Render("OpenRouter API key")
	body := m.keyInput.View()
	hint := "Enter to continue, b to go back."
	return lipgloss.JoinVertical(lipgloss.Top, header, body, "", hint)
}

func (m Model) renderConfigView() string {
	lines := []string{
		fmt.Sprintf("Base branch: %s", m.baseBranch),
		fmt.Sprintf("Review branch: %s", m.branch),
	}
	if m.reviewResult.Model != "" {
		lines = append(lines, fmt.Sprintf("Model: %s", m.reviewResult.Model))
	} else if m.cfg.LastModel != "" {
		lines = append(lines, fmt.Sprintf("Model: %s", m.cfg.LastModel))
	} else {
		lines = append(lines, fmt.Sprintf("Model: %s", review.DefaultModel))
	}
	if m.guidelineHash == "" {
		lines = append(lines, "Guideline hash: (none)")
	} else {
		lines = append(lines, fmt.Sprintf("Guideline hash: %s", m.guidelineHash))
	}

	if len(m.cfg.Guidelines) == 0 {
		lines = append(lines, "Guidelines: none")
	} else {
		lines = append(lines, "Guidelines:")
		for _, path := range m.cfg.Guidelines {
			lines = append(lines, "- "+m.formatGuidelineLabel(path))
		}
	}

	if m.cfg.FreeGuideline != "" {
		lines = append(lines, "", "Free-text guideline:", m.cfg.FreeGuideline)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderCommentsView() string {
	if m.reviewRunning {
		return m.renderReviewStatus("Reviewing comments...")
	}
	if len(m.reviewResult.Comments) == 0 {
		if m.reviewResult.Dropped > 0 || len(m.reviewResult.FileErrors) > 0 {
			return lipgloss.JoinVertical(
				lipgloss.Top,
				m.renderCommentsWarnings(),
				"No comments generated.",
				"",
				m.renderCommentsHints(),
			)
		}
		return "No comments generated."
	}
	if len(m.commentsIndexMap) == 0 {
		return lipgloss.JoinVertical(
			lipgloss.Top,
			m.renderCommentsWarnings(),
			m.renderCommentsFilters(),
			"No comments match current filters.",
			"",
			m.renderCommentsHints(),
		)
	}

	leftWidth, rightWidth := m.commentsPaneWidths()

	focusedStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	unfocusedStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("241"))

	leftPaneStyle := unfocusedStyle
	rightPaneStyle := unfocusedStyle

	if m.commentsPanelFocus == panelFocusLeft {
		leftPaneStyle = focusedStyle
	} else {
		rightPaneStyle = focusedStyle
	}

	tableView := m.commentsTable.View()
	detailView := m.commentsDetailView.View()
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		leftPaneStyle.Width(leftWidth).Render(tableView),
		rightPaneStyle.Width(rightWidth).Render(detailView),
	)

	return lipgloss.JoinVertical(lipgloss.Top, m.renderCommentsWarnings(), m.renderCommentsFilters(), panes, "", m.renderCommentsHints())
}

func (m Model) renderVerdictView() string {
	if m.reviewRunning {
		return m.renderReviewStatus("Reviewing verdict...")
	}
	if m.reviewResult.Verdict.Decision == "" {
		return "Verdict not available."
	}

	verdict := m.reviewResult.Verdict
	lines := []string{
		fmt.Sprintf("Decision: %s", verdict.Decision),
		fmt.Sprintf("Summary: %s", verdict.Summary),
	}
	if len(verdict.Rationale) > 0 {
		lines = append(lines, "", "Rationale:")
		for _, item := range verdict.Rationale {
			lines = append(lines, "- "+item)
		}
	}
	lines = append(lines, "", fmt.Sprintf("Stats: NIT=%d, SUGGESTION=%d, ISSUE=%d, BLOCKER=%d", verdict.Stats.Nit, verdict.Stats.Suggestion, verdict.Stats.Issue, verdict.Stats.Blocker))
	return strings.Join(lines, "\n")
}

func (m Model) renderReviewStatus(heading string) string {
	if m.reviewProgress.total == 0 {
		return heading
	}
	status := fmt.Sprintf("%s (%d/%d, failed %d)", heading, m.reviewProgress.completed, m.reviewProgress.total, m.reviewProgress.failed)
	if m.reviewProgress.file != "" {
		last := "ok"
		if m.reviewProgress.lastError != "" {
			last = "error"
		}
		status = fmt.Sprintf("%s: %s (%s)", status, m.reviewProgress.file, last)
		if m.reviewProgress.lastError != "" {
			status = fmt.Sprintf("%s - %s", status, shortenMessage(m.reviewProgress.lastError, m.width-2))
		}
	}
	return status
}

func (m *Model) updateDiffViewportLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	leftWidth, rightWidth := m.diffPaneWidths()
	_ = leftWidth
	m.diffView.Width = rightWidth - 2
	m.diffView.Height = m.diffPaneHeight() - 2
	m.diffView.SetYOffset(m.diffView.YOffset)
}

func (m *Model) updateDiffViewportContent() {
	m.diffView.SetContent(m.renderFileDiff())
	m.diffView.SetYOffset(0)
}

func (m Model) diffPaneWidths() (int, int) {
	leftWidth := int(float64(m.width) * 0.3)
	if leftWidth < 20 {
		leftWidth = 20
	}
	rightWidth := m.width - leftWidth - 1
	if rightWidth < 20 {
		rightWidth = 20
	}
	return leftWidth, rightWidth
}

func (m Model) diffPaneHeight() int {
	height := m.height - 2
	if height < 5 {
		height = 5
	}
	return height
}

func (m *Model) updateCommentsTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.commentsFilterActive {
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc", "enter":
			m.commentsFilterActive = false
			m.commentsFileFilter.Blur()
			m.commentsTable.Focus()
			return m, nil
		default:
			var cmd tea.Cmd
			m.commentsFileFilter, cmd = m.commentsFileFilter.Update(msg)
			m.refreshCommentsTable()
			return m, cmd
		}
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "right", "l":
		m.active = (m.active + 1) % len(m.tabs)
		return m, nil
	case "left", "h":
		m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
		return m, nil
	case "tab":
		m.toggleCommentsPanelFocus()
		return m, nil
	case "/":
		m.commentsFilterActive = true
		m.commentsFileFilter.Focus()
		m.commentsTable.Blur()
		return m, nil
	case "s":
		m.cycleSeverityFilter()
		m.refreshCommentsTable()
		return m, nil
	case "c":
		m.commentsSeverityFilter = ""
		m.commentsFileFilter.SetValue("")
		m.refreshCommentsTable()
		return m, nil
	case " ":
		if m.commentsPanelFocus == panelFocusLeft {
			m.toggleSelectedCommentPublish()
			m.refreshCommentsTable()
			return m, nil
		}
	case "r":
		m.reviewResult = review.Result{}
		m.reviewRunning = true
		m.reviewProgress = reviewProgressMsg{}
		return m, m.maybeStartReview()
	}

	if m.commentsPanelFocus == panelFocusRight {
		var cmd tea.Cmd
		m.commentsDetailView, cmd = m.commentsDetailView.Update(msg)
		return m, cmd
	}

	beforeIndex, _ := m.selectedCommentIndex()
	var cmd tea.Cmd
	m.commentsTable, cmd = m.commentsTable.Update(msg)
	afterIndex, _ := m.selectedCommentIndex()
	if beforeIndex != afterIndex {
		m.updateCommentsDetailContent(true)
	}
	return m, cmd
}

func (m *Model) cycleSeverityFilter() {
	sequence := []review.Severity{
		"",
		review.SeverityBlocker,
		review.SeverityIssue,
		review.SeveritySuggestion,
		review.SeverityNit,
	}
	current := 0
	for i, value := range sequence {
		if value == m.commentsSeverityFilter {
			current = i
			break
		}
	}
	next := (current + 1) % len(sequence)
	m.commentsSeverityFilter = sequence[next]
}

func (m *Model) toggleSelectedCommentPublish() {
	index, ok := m.selectedCommentIndex()
	if !ok {
		return
	}
	current := m.reviewResult.Comments[index]
	current.Publish = !current.Publish
	m.reviewResult.Comments[index] = current
}

func (m *Model) refreshCommentsTable() {
	rows, indices := m.buildCommentRows()
	m.commentsIndexMap = indices
	m.commentsTable.SetRows(rows)
	if len(rows) == 0 {
		m.commentsTable.SetCursor(0)
		m.updateCommentsDetailContent(true)
		return
	}
	if m.commentsTable.Cursor() >= len(rows) {
		m.commentsTable.SetCursor(len(rows) - 1)
	}
	m.updateCommentsDetailContent(true)
}

func (m *Model) updatePublishTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.publishRunning {
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "right", "l":
		m.active = (m.active + 1) % len(m.tabs)
		m.blurPublishInputs()
		m.focusPublishInput()
		return m, nil
	case "left", "h":
		m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
		m.blurPublishInputs()
		m.focusPublishInput()
		return m, nil
	case "tab":
		m.cyclePublishFocus()
		return m, nil
	case "enter":
		if m.publishWorkspaceInput.Focused() || m.publishRepoSlugInput.Focused() || m.publishPRIDInput.Focused() || m.publishTokenInput.Focused() {
			m.cyclePublishFocus()
			return m, nil
		}
	case "p":
		if !m.publishWorkspaceInput.Focused() && !m.publishRepoSlugInput.Focused() && !m.publishPRIDInput.Focused() && !m.publishTokenInput.Focused() {
			ctx, cancel := context.WithCancel(context.Background())
			return m, tea.Batch(
				func() tea.Msg { return publishStartedMsg{cancel: cancel} },
				m.publishReviewCmd(ctx),
			)
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.publishWorkspaceInput, cmd = m.publishWorkspaceInput.Update(msg)
	cmds = append(cmds, cmd)

	m.publishRepoSlugInput, cmd = m.publishRepoSlugInput.Update(msg)
	cmds = append(cmds, cmd)

	m.publishPRIDInput, cmd = m.publishPRIDInput.Update(msg)
	cmds = append(cmds, cmd)

	m.publishTokenInput, cmd = m.publishTokenInput.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) blurPublishInputs() {
	m.publishWorkspaceInput.Blur()
	m.publishRepoSlugInput.Blur()
	m.publishPRIDInput.Blur()
	m.publishTokenInput.Blur()
}

func (m *Model) focusPublishInput() {
	if m.tabs[m.active] == "Publish" {
		m.publishWorkspaceInput.Focus()
	}
}

func (m *Model) cyclePublishFocus() {
	if m.publishWorkspaceInput.Focused() {
		m.publishWorkspaceInput.Blur()
		m.publishRepoSlugInput.Focus()
	} else if m.publishRepoSlugInput.Focused() {
		m.publishRepoSlugInput.Blur()
		m.publishPRIDInput.Focus()
	} else if m.publishPRIDInput.Focused() {
		m.publishPRIDInput.Blur()
		if config.BitbucketToken() == "" {
			m.publishTokenInput.Focus()
		} else {
			m.publishWorkspaceInput.Focus()
		}
	} else if m.publishTokenInput.Focused() {
		m.publishTokenInput.Blur()
		m.publishWorkspaceInput.Focus()
	} else {
		m.publishWorkspaceInput.Focus()
	}
}

func (m *Model) updateCommentsTableLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	leftWidth, rightWidth := m.commentsPaneWidths()
	m.commentsTableWidth = leftWidth
	height := m.height - 9
	if height < 6 {
		height = 6
	}
	m.commentsTableHeight = height
	m.commentsTable.SetWidth(leftWidth - 2)
	m.commentsTable.SetHeight(height - 2)
	m.commentsDetailView.Width = rightWidth - 2
	m.commentsDetailView.Height = height - 2
	m.updateCommentsDetailContent(false)

	available := leftWidth - 28
	if available < 20 {
		available = 20
	}
	fileWidth := int(float64(available) * 0.4)
	if fileWidth < 10 {
		fileWidth = 10
	}
	titleWidth := available - fileWidth
	if titleWidth < 10 {
		titleWidth = 10
	}
	cols := []table.Column{
		{Title: "Sev", Width: 9},
		{Title: "File", Width: fileWidth},
		{Title: "Line", Width: 8},
		{Title: "Title", Width: titleWidth},
		{Title: "Pub", Width: 5},
	}
	m.commentsTable.SetColumns(cols)
}

func (m Model) buildCommentRows() ([]table.Row, []int) {
	rows := make([]table.Row, 0, len(m.reviewResult.Comments))
	indices := make([]int, 0, len(m.reviewResult.Comments))
	fileFilter := strings.ToLower(strings.TrimSpace(m.commentsFileFilter.Value()))

	for i, comment := range m.reviewResult.Comments {
		if m.commentsSeverityFilter != "" && comment.Severity != m.commentsSeverityFilter {
			continue
		}
		if fileFilter != "" && !strings.Contains(strings.ToLower(comment.FilePath), fileFilter) {
			continue
		}
		line := fmt.Sprintf("%d", comment.StartLine)
		if comment.EndLine > comment.StartLine {
			line = fmt.Sprintf("%d-%d", comment.StartLine, comment.EndLine)
		}
		publish := "yes"
		if !comment.Publish {
			publish = "no"
		}
		rows = append(rows, table.Row{
			string(comment.Severity),
			comment.FilePath,
			line,
			comment.Title,
			publish,
		})
		indices = append(indices, i)
	}
	return rows, indices
}

func (m Model) selectedCommentIndex() (int, bool) {
	if len(m.commentsIndexMap) == 0 {
		return 0, false
	}
	cursor := m.commentsTable.Cursor()
	if cursor < 0 || cursor >= len(m.commentsIndexMap) {
		return 0, false
	}
	return m.commentsIndexMap[cursor], true
}

func (m Model) renderCommentDetailContent(width int) string {
	index, ok := m.selectedCommentIndex()
	if !ok {
		return "No comment selected."
	}
	comment := m.reviewResult.Comments[index]
	lineRange := fmt.Sprintf("%d", comment.StartLine)
	if comment.EndLine > comment.StartLine {
		lineRange = fmt.Sprintf("%d-%d", comment.StartLine, comment.EndLine)
	}
	publishLabel := "included"
	if !comment.Publish {
		publishLabel = "excluded"
	}
	lines := []string{
		fmt.Sprintf("Severity: %s", comment.Severity),
		fmt.Sprintf("File: %s", comment.FilePath),
		fmt.Sprintf("Lines: %s", lineRange),
		fmt.Sprintf("Publish: %s", publishLabel),
		"",
		"Title:",
		comment.Title,
		"",
		"Body:",
		comment.Body,
	}
	if comment.Suggestion != nil && strings.TrimSpace(*comment.Suggestion) != "" {
		lines = append(lines, "", "Suggestion:", *comment.Suggestion)
	}
	if comment.Evidence != nil && strings.TrimSpace(*comment.Evidence) != "" {
		lines = append(lines, "", "Evidence:", *comment.Evidence)
	}
	if len(comment.Tags) > 0 {
		lines = append(lines, "", "Tags:", strings.Join(comment.Tags, ", "))
	}
	content := strings.Join(lines, "\n")
	if width <= 0 {
		return content
	}
	return lipgloss.NewStyle().Width(width).Render(content)
}

func (m Model) renderCommentsFilters() string {
	severity := "ALL"
	if m.commentsSeverityFilter != "" {
		severity = string(m.commentsSeverityFilter)
	}
	fileValue := strings.TrimSpace(m.commentsFileFilter.Value())
	if m.commentsFilterActive {
		fileValue = m.commentsFileFilter.View()
	} else if fileValue == "" {
		fileValue = "(none)"
	}
	return fmt.Sprintf("Severity: %s | File: %s", severity, fileValue)
}

func (m Model) renderCommentsWarnings() string {
	warnings := make([]string, 0)
	if m.reviewResult.Dropped > 0 {
		warnings = append(warnings, fmt.Sprintf("Warning: %d comment(s) dropped due to missing file/line/title/body.", m.reviewResult.Dropped))
	}
	if len(m.reviewResult.FileErrors) > 0 {
		var failedFiles []string
		for path := range m.reviewResult.FileErrors {
			failedFiles = append(failedFiles, filepath.Base(path))
		}
		sort.Strings(failedFiles)
		warnings = append(warnings, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(
			fmt.Sprintf("Failed to review %d file(s): %s", len(failedFiles), strings.Join(failedFiles, ", ")),
		))
	}
	if len(warnings) == 0 {
		return ""
	}
	return strings.Join(warnings, "\n")
}

func (m Model) renderCommentsHints() string {
	hints := []string{
		"↑/↓ to move, Space to toggle publish, s to cycle severity, / to filter file, c to clear filters, Tab to switch panel.",
	}
	if m.commentsFilterActive {
		hints = []string{"Typing filter... Enter/Esc to apply."}
	}
	return strings.Join(hints, "\n")
}

func (m Model) selectedGuidelines() []string {
	paths := make([]string, 0, len(m.guidelineSelected))
	for path, selected := range m.guidelineSelected {
		if selected {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func (m Model) formatGuidelineLabel(path string) string {
	rel, err := filepath.Rel(m.repoRoot, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func (m Model) hasGuidelineOption(path string) bool {
	for _, option := range m.guidelineOptions {
		if option == path {
			return true
		}
	}
	return false
}

func validateGuidelinePath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("guideline path is not a file")
	}
	if strings.ToLower(filepath.Ext(path)) != ".md" {
		return fmt.Errorf("guideline must be a .md file")
	}
	return nil
}

func clamp(value, min, max int) int {
	if max < min {
		return min
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func (m Model) filteredBranches() []string {
	filter := strings.ToLower(strings.TrimSpace(m.branchFilterInput.Value()))
	if filter == "" {
		return m.branches
	}

	filtered := make([]string, 0, len(m.branches))
	for _, branch := range m.branches {
		if strings.Contains(strings.ToLower(branch), filter) {
			filtered = append(filtered, branch)
		}
	}
	return filtered
}

func (m Model) branchVisibleCount() int {
	if m.height == 0 {
		return 10
	}
	usable := m.height - 8
	if usable < 5 {
		return 5
	}
	return usable
}

func clampWindow(cursor, total, window int) (int, int) {
	if window <= 0 {
		window = 1
	}
	if total <= window {
		return 0, total
	}
	start := cursor - window/2
	if start < 0 {
		start = 0
	}
	end := start + window
	if end > total {
		end = total
		start = end - window
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func shortenMessage(message string, width int) string {
	trimmed := strings.TrimSpace(message)
	if width <= 0 || len(trimmed) <= width {
		return trimmed
	}
	if width <= 3 {
		return trimmed[:width]
	}
	return trimmed[:width-3] + "..."
}

func (m *Model) updateCommentsDetailContent(reset bool) {
	width := m.commentsDetailView.Width
	m.commentsDetailView.SetContent(m.renderCommentDetailContent(width))
	if reset {
		m.commentsDetailView.SetYOffset(0)
	}
}

func (m Model) commentsPaneWidths() (int, int) {
	leftWidth := int(float64(m.width) * 0.55)
	if leftWidth < 40 {
		leftWidth = 40
	}
	rightWidth := m.width - leftWidth - 1
	if rightWidth < 24 {
		rightWidth = 24
	}
	return leftWidth, rightWidth
}

func (m *Model) toggleCommentsPanelFocus() {
	if m.commentsPanelFocus == panelFocusLeft {
		m.commentsPanelFocus = panelFocusRight
		m.commentsTable.Blur()
		return
	}
	m.commentsPanelFocus = panelFocusLeft
	m.commentsTable.Focus()
}

func (m *Model) updateDiffTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "right", "l":
		m.active = (m.active + 1) % len(m.tabs)
		return m, nil
	case "left", "h":
		m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
		return m, nil
	case "tab":
		if m.diffPanelFocus == panelFocusLeft {
			m.diffPanelFocus = panelFocusRight
		} else {
			m.diffPanelFocus = panelFocusLeft
		}
		return m, nil
	}

	if m.diffPanelFocus == panelFocusRight {
		switch msg.String() {
		case "pgdown", "ctrl+d":
			m.diffView.PageDown()
			return m, nil
		case "pgup", "ctrl+u":
			m.diffView.PageUp()
			return m, nil
		}
		var cmd tea.Cmd
		m.diffView, cmd = m.diffView.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "up", "k":
		m.diffFile = clamp(m.diffFile-1, 0, len(m.diffFiles)-1)
		m.updateDiffViewportContent()
		return m, nil
	case "down", "j":
		m.diffFile = clamp(m.diffFile+1, 0, len(m.diffFiles)-1)
		m.updateDiffViewportContent()
		return m, nil
	}

	return m, nil
}

func (m Model) maybeStartReview() tea.Cmd {
	if m.reviewRunning || !m.reviewResult.GeneratedAt.IsZero() {
		return nil
	}
	if len(m.diffFiles) == 0 || m.diffErr != nil {
		return nil
	}
	apiKey := strings.TrimSpace(m.openRouterKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(config.OpenRouterAPIKey())
	}
	if apiKey == "" {
		m.reviewErr = errors.New("missing OPENROUTER_API_KEY")
		return nil
	}
	return startReviewCmd(m.diffFiles, m.cfg, m.guidelineHash, apiKey)
}

func startReviewCmd(diffFiles []git.DiffFile, cfg config.Config, guidelineHash string, apiKey string) tea.Cmd {
	return func() tea.Msg {
		slog.Info("Starting review", "files", len(diffFiles), "model", cfg.LastModel, "hash", guidelineHash)
		updates := make(chan tea.Msg)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			defer close(updates)
			updates <- reviewProgressMsg{completed: 0, total: len(diffFiles), failed: 0, file: "starting"}
			client := llm.NewClient(apiKey, config.OpenRouterBaseURL())
			result, err := review.Run(ctx, client, diffFiles, review.RunOptions{
				Model:          cfg.LastModel,
				GuidelinePaths: cfg.Guidelines,
				FreeText:       cfg.FreeGuideline,
				GuidelineHash:  guidelineHash,
			}, func(progress review.Progress) {
				select {
				case <-ctx.Done():
					return
				case updates <- reviewProgressMsg{
					completed: progress.Completed,
					total:     progress.Total,
					failed:    progress.Failed,
					file:      progress.CurrentFile,
					lastError: progress.LastError,
				}:
				}
			})
			select {
			case <-ctx.Done():
				return
			default:
				updates <- reviewCompletedMsg{result: result, err: err}
			}
		}()
		return reviewStartedMsg{updates: updates, cancel: cancel}
	}
}

func listenReviewCmd(updates <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-updates
		if !ok {
			return nil
		}
		return msg
	}
}

func (m Model) publishReviewCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		slog.Info("Starting publish to Bitbucket")
		token := strings.TrimSpace(m.publishToken)
		if token == "" {
			token = strings.TrimSpace(config.BitbucketToken())
		}

		workspace := strings.TrimSpace(m.publishWorkspaceInput.Value())
		repoSlug := strings.TrimSpace(m.publishRepoSlugInput.Value())
		prIDStr := strings.TrimSpace(m.publishPRIDInput.Value())

		var prID int
		fmt.Sscanf(prIDStr, "%d", &prID)

		if token == "" || workspace == "" || repoSlug == "" || prID == 0 {
			return publishCompletedMsg{err: errors.New("missing bitbucket configuration (workspace, repo, PR ID, or token)")}
		}

		cfg := bitbucket.Config{
			Workspace:   workspace,
			RepoSlug:    repoSlug,
			PullRequest: prID,
			Token:       token,
		}

		client := bitbucket.NewClient(cfg)
		markdown := bitbucket.ComposeMarkdown(m.reviewResult)

		resultID, err := client.PublishComment(ctx, markdown)
		return publishCompletedMsg{resultID: resultID, err: err}
	}
}

func (m Model) renderErrorView(err error, hint string) string {
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true).
		Padding(0, 0, 1, 0)

	background := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("9"))

	content := lipgloss.JoinVertical(lipgloss.Left,
		errorStyle.Render("ERROR"),
		m.wrapText(err.Error(), m.width/2),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(hint),
	)

	return lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, background.Render(content))
}

func (m Model) wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	return lipgloss.NewStyle().Width(width).Render(text)
}

func (m Model) renderStatusBar() string {
	w := m.width
	if w <= 0 {
		return ""
	}

	mode := "DASHBOARD"
	if m.inWizard {
		mode = "WIZARD"
	}

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#C1C1C1")).
		Background(lipgloss.Color("#353535")).
		Padding(0, 1)

	modeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#6124DF")).
		Bold(true).
		Padding(0, 1)

	status := "q: quit • ?: help • h/l: tabs"
	if m.inWizard {
		status = "q: quit • enter: next • b: back"
	}

	modeStr := modeStyle.Render(mode)
	statusStr := style.Width(w - lipgloss.Width(modeStr)).Render(status)

	return lipgloss.JoinHorizontal(lipgloss.Top, modeStr, statusStr)
}

func (m Model) renderHelpOverlay(_ string) string {
	helpText := `KEYBOARD SHORTCUTS

Global:
q, ctrl+c   Quit
?           Toggle help
h, left     Previous tab
l, right    Next tab

Diff Tab:
j, down     Next file
k, up       Previous file
tab         Switch between file list and diff
pgup, pgdn  Scroll diff (when focused)

Comments Tab:
j, down     Next comment
k, up       Previous comment
r           Retry review
space       Toggle publish inclusion
s           Cycle severity filter
/           Search by file path
c           Clear filters
tab         Switch between table and detail

Publish Tab:
tab         Cycle input fields
p           Execute publishing

Config Tab:
r           Re-run review (keep config)

Press any key to close help.`

	overlayStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Background(lipgloss.Color("#1A1A1A"))

	overlay := overlayStyle.Render(helpText)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}
