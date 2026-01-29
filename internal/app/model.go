package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	tabs   []string
	active int
	width  int
	height int
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
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "right", "l":
			m.active = (m.active + 1) % len(m.tabs)
			return m, nil
		case "left", "h":
			m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
			return m, nil
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	tabLine := m.renderTabs()
	content := fmt.Sprintf("%s view\n\nComing soon.", m.tabs[m.active])

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
