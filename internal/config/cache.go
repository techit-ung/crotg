package config

import (
	"os"
	"path/filepath"
)

func CacheDir() (string, error) {
	baseDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "reviewer"), nil
}
