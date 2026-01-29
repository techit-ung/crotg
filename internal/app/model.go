package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

	guidelineOptions  []string
	guidelineSelected map[string]bool
	guidelineCursor   int
	guidelineErr      error
	guidelineHash     string
	pathInput         textinput.Model
	freeTextInput     textinput.Model
	keyInput          textinput.Model
	openRouterKey     string
	branchFilterInput textinput.Model

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
}

func NewModel() Model {
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
	commentsFileFilter := textinput.New()
	commentsFileFilter.Placeholder = "Filter by file path"
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
		commentsFileFilter: commentsFileFilter,
		commentsTable:      commentsTable,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadConfigCmd(), detectRepoCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case configLoadedMsg:
		m.cfg = msg.cfg
		return m, nil
	case configSavedMsg:
		return m, nil
	case diffLoadedMsg:
		m.diffText = msg.raw
		m.diffFiles = msg.files
		m.diffErr = msg.err
		if msg.err == nil {
			return m, m.maybeStartReview()
		}
		return m, nil
	case guidelinesScannedMsg:
		m.guidelineOptions = msg.paths
		m.guidelineErr = msg.err
		m.guidelineSelected = make(map[string]bool)
		for _, path := range msg.paths {
			for _, selected := range m.cfg.Guidelines {
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
		if msg.err == nil {
			m.reviewResult = msg.result
			m.refreshCommentsTable()
			m.updateCommentsTableLayout()
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
		m.updateCommentsTableLayout()
		return m, nil
	case tea.KeyMsg:
		if m.inWizard {
			return m.updateWizard(msg)
		}
		if m.tabs[m.active] == "Comments" {
			return m.updateCommentsTab(msg)
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
		case "up", "k":
			if m.tabs[m.active] == "Diff" {
				m.diffFile = clamp(m.diffFile-1, 0, len(m.diffFiles)-1)
				return m, nil
			}
		case "down", "j":
			if m.tabs[m.active] == "Diff" {
				m.diffFile = clamp(m.diffFile+1, 0, len(m.diffFiles)-1)
				return m, nil
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	if m.inWizard {
		return m.renderWizard()
	}

	tabLine := m.renderTabs()
	content := m.renderActiveView()

	return lipgloss.JoinVertical(lipgloss.Top, tabLine, content)
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
	wizardGuidelines
	wizardGuidelinePath
	wizardFreeGuideline
	wizardOpenRouterKey
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
}

type reviewProgressMsg struct {
	completed int
	total     int
	file      string
}

type reviewCompletedMsg struct {
	result review.Result
	err    error
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
			m.wizardStep = wizardGuidelines
			m.guidelineCursor = 0
			m.guidelineErr = nil
			m.branchFilterInput.Blur()
			return m, scanGuidelinesCmd(m.repoRoot, m.cfg.Guidelines)
		default:
			var cmd tea.Cmd
			m.branchFilterInput, cmd = m.branchFilterInput.Update(msg)
			m.cursor = 0
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
	header := lipgloss.NewStyle().Bold(true).Render("Setup Wizard")
	if m.err != nil {
		body := fmt.Sprintf("Error: %s\n\nPress r to retry, q to quit.", m.err)
		return lipgloss.JoinVertical(lipgloss.Top, header, body)
	}

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
	switch m.tabs[m.active] {
	case "Diff":
		return m.renderDiffView()
	case "Comments":
		return m.renderCommentsView()
	case "Verdict":
		return m.renderVerdictView()
	case "Config":
		return m.renderConfigView()
	default:
		return fmt.Sprintf("%s view\n\nComing soon.", m.tabs[m.active])
	}
}

func (m Model) renderDiffView() string {
	if m.diffErr != nil {
		return fmt.Sprintf("Diff error: %s", m.diffErr)
	}
	if len(m.diffFiles) == 0 {
		return "No diff files to display."
	}

	leftWidth := int(float64(m.width) * 0.3)
	if leftWidth < 20 {
		leftWidth = 20
	}
	rightWidth := m.width - leftWidth - 1
	if rightWidth < 20 {
		rightWidth = 20
	}

	left := lipgloss.NewStyle().Width(leftWidth).PaddingRight(1)
	right := lipgloss.NewStyle().Width(rightWidth)

	fileList := m.renderFileList(leftWidth)
	diffPane := m.renderFileDiff(rightWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, left.Render(fileList), right.Render(diffPane))
}

func (m Model) renderFileList(width int) string {
	lines := make([]string, 0, len(m.diffFiles))
	for i, file := range m.diffFiles {
		cursor := "  "
		if i == m.diffFile {
			cursor = "> "
		}
		lines = append(lines, cursor+file.Path)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderFileDiff(width int) string {
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
	if m.reviewErr != nil {
		return fmt.Sprintf("Review error: %s", m.reviewErr)
	}
	if len(m.reviewResult.Comments) == 0 {
		return "No comments generated."
	}
	if len(m.commentsIndexMap) == 0 {
		return lipgloss.JoinVertical(
			lipgloss.Top,
			m.renderCommentsFilters(),
			"No comments match current filters.",
		)
	}

	leftWidth := m.commentsTableWidth
	if leftWidth == 0 {
		leftWidth = int(float64(m.width) * 0.55)
		if leftWidth < 40 {
			leftWidth = 40
		}
	}
	rightWidth := m.width - leftWidth - 1
	if rightWidth < 24 {
		rightWidth = 24
	}

	left := lipgloss.NewStyle().Width(leftWidth).PaddingRight(1)
	right := lipgloss.NewStyle().Width(rightWidth)

	tableView := m.commentsTable.View()
	detailView := m.renderCommentDetail(rightWidth)
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left.Render(tableView), right.Render(detailView))

	return lipgloss.JoinVertical(lipgloss.Top, m.renderCommentsFilters(), panes, "", m.renderCommentsHints())
}

func (m Model) renderVerdictView() string {
	if m.reviewRunning {
		return m.renderReviewStatus("Reviewing verdict...")
	}
	if m.reviewErr != nil {
		return fmt.Sprintf("Review error: %s", m.reviewErr)
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
	status := fmt.Sprintf("%s (%d/%d)", heading, m.reviewProgress.completed, m.reviewProgress.total)
	if m.reviewProgress.file != "" {
		status = fmt.Sprintf("%s: %s", status, m.reviewProgress.file)
	}
	return status
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
		m.toggleSelectedCommentPublish()
		m.refreshCommentsTable()
		return m, nil
	}

	var cmd tea.Cmd
	m.commentsTable, cmd = m.commentsTable.Update(msg)
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
		return
	}
	if m.commentsTable.Cursor() >= len(rows) {
		m.commentsTable.SetCursor(len(rows) - 1)
	}
}

func (m *Model) updateCommentsTableLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	leftWidth := int(float64(m.width) * 0.55)
	if leftWidth < 40 {
		leftWidth = 40
	}
	m.commentsTableWidth = leftWidth
	height := m.height - 6
	if height < 6 {
		height = 6
	}
	m.commentsTableHeight = height
	m.commentsTable.SetWidth(leftWidth)
	m.commentsTable.SetHeight(height)

	available := leftWidth - 26
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

func (m Model) renderCommentDetail(width int) string {
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

func (m Model) renderCommentsHints() string {
	hints := []string{
		"↑/↓ to move, Space to toggle publish, s to cycle severity, / to filter file, c to clear filters.",
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

func (m Model) maybeStartReview() tea.Cmd {
	if m.reviewRunning || m.reviewResult.GeneratedAt.Unix() != 0 {
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
		updates := make(chan tea.Msg)
		go func() {
			defer close(updates)
			client := llm.NewClient(apiKey, config.OpenRouterBaseURL())
			ctx := context.Background()
			result, err := review.Run(ctx, client, diffFiles, review.RunOptions{
				Model:          cfg.LastModel,
				GuidelinePaths: cfg.Guidelines,
				FreeText:       cfg.FreeGuideline,
				GuidelineHash:  guidelineHash,
			}, func(progress review.Progress) {
				updates <- reviewProgressMsg{completed: progress.Completed, total: progress.Total, file: progress.CurrentFile}
			})
			updates <- reviewCompletedMsg{result: result, err: err}
		}()
		return reviewStartedMsg{updates: updates}
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
