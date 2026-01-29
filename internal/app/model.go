package app

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/config"
	"github.com/techitung-arunyawee/code-reviewer-2/internal/git"
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
}

func NewModel() Model {
	return Model{
		tabs: []string{
			"Diff",
			"Comments",
			"Verdict",
			"Publish",
			"Config",
		},
		inWizard:   true,
		wizardStep: wizardRepo,
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
			m.inWizard = false
			m.cfg.LastBase = m.baseBranch
			m.cfg.LastBranch = m.branch
			return m, tea.Batch(saveConfigCmd(m.cfg), generateDiffCmd(m.repoRoot, m.baseBranch, m.branch))
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
	default:
		return "loading..."
	}
}

func (m Model) renderActiveView() string {
	switch m.tabs[m.active] {
	case "Diff":
		return m.renderDiffView()
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
