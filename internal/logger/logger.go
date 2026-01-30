package logger

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/config"
)

func Init(debug bool) (*os.File, error) {
	configDir, err := config.ConfigDir()
	if err != nil {
		return nil, err
	}

	logDir := filepath.Join(configDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	logFile := filepath.Join(logDir, "reviewer.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	logger := slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: level,
	}))

	slog.SetDefault(logger)
	return f, nil
}
