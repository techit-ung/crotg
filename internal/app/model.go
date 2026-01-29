package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/config"
	"github.com/techitung-arunyawee/code-reviewer-2/internal/git"
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
}

func NewModel() Model {
	pathInput := textinput.New()
	pathInput.Placeholder = "path/to/guideline.md"
	freeTextInput := textinput.New()
	freeTextInput.Placeholder = "Free-text guideline (optional)"

	return Model{
		tabs: []string{
			"Diff",
			"Comments",
			"Verdict",
			"Publish",
			"Config",
		},
		inWizard:      true,
		wizardStep:    wizardRepo,
		pathInput:     pathInput,
		freeTextInput: freeTextInput,
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
		return m, nil
	case tea.KeyMsg:
		if m.inWizard {
			return m.updateWizard(msg)
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
		}
	case wizardBaseBranch:
		switch msg.String() {
		case "up", "k":
			m.cursor = clamp(m.cursor-1, 0, len(m.branches)-1)
		case "down", "j":
			m.cursor = clamp(m.cursor+1, 0, len(m.branches)-1)
		case "enter":
			if len(m.branches) == 0 {
				return m, nil
			}
			m.baseBranch = m.branches[m.cursor]
			m.wizardStep = wizardBranch
			m.cursor = m.initialBranchIndex(m.cfg.LastBranch)
		}
	case wizardBranch:
		switch msg.String() {
		case "up", "k":
			m.cursor = clamp(m.cursor-1, 0, len(m.branches)-1)
		case "down", "j":
			m.cursor = clamp(m.cursor+1, 0, len(m.branches)-1)
		case "b":
			m.wizardStep = wizardBaseBranch
			m.cursor = m.initialBranchIndex(m.baseBranch)
		case "enter":
			if len(m.branches) == 0 {
				return m, nil
			}
			m.branch = m.branches[m.cursor]
			m.wizardStep = wizardGuidelines
			m.guidelineCursor = 0
			m.guidelineErr = nil
			return m, scanGuidelinesCmd(m.repoRoot, m.cfg.Guidelines)
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
	default:
		return "loading..."
	}
}

func (m Model) renderActiveView() string {
	switch m.tabs[m.active] {
	case "Diff":
		return m.renderDiffView()
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

	lines := make([]string, 0, len(m.branches))
	for i, branch := range m.branches {
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

	hint := "Use ↑/↓ and Enter."
	if m.wizardStep == wizardBranch {
		hint = "Use ↑/↓ and Enter. Press b to go back."
	}

	return lipgloss.JoinVertical(lipgloss.Top, header, strings.Join(lines, "\n"), "", hint)
}

func (m Model) initialBranchIndex(branch string) int {
	if branch == "" {
		return 0
	}
	for i, name := range m.branches {
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

func (m Model) renderConfigView() string {
	lines := []string{
		fmt.Sprintf("Base branch: %s", m.baseBranch),
		fmt.Sprintf("Review branch: %s", m.branch),
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
