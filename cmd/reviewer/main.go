package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/app"
)

func main() {
	program := tea.NewProgram(app.NewModel(), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}
}
