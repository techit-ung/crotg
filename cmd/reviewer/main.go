package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/app"
	"github.com/techitung-arunyawee/code-reviewer-2/internal/logger"
)

func main() {
	debug := flag.Bool("debug", false, "Enable debug logging")
	version := flag.Bool("version", false, "Show version")
	base := flag.String("base", "", "Base branch")
	branch := flag.String("branch", "", "Review branch")
	model := flag.String("model", "", "Model name")
	guideline := flag.String("guideline", "", "Guideline profile path")
	flag.Parse()

	if *version {
		fmt.Println("reviewer version v0.1.0")
		os.Exit(0)
	}

	logFile, err := logger.Init(*debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	program := tea.NewProgram(app.NewModel(*base, *branch, *model, *guideline), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}
}
