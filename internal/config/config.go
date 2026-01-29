package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	LastBranch    string   `json:"lastBranch,omitempty"`
	LastBase      string   `json:"lastBase,omitempty"`
	LastModel     string   `json:"lastModel,omitempty"`
	Guidelines    []string `json:"guidelines,omitempty"`
	FreeGuideline string   `json:"freeGuideline,omitempty"`
}

func ConfigDir() (string, error) {
	if dir := os.Getenv("CODE_REVIEWER_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	baseDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(baseDir, "reviewer"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.json"), nil
}

func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}
