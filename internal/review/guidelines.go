package review

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ScanGuidelineFiles(repoRoot string, extraPaths []string) ([]string, error) {
	seen := make(map[string]struct{})

	addPath := func(path string) {
		if path == "" {
			return
		}
		seen[path] = struct{}{}
	}

	rootProfile := filepath.Join(repoRoot, ".review.md")
	if isRegularFile(rootProfile) {
		addPath(rootProfile)
	}

	profileDir := filepath.Join(repoRoot, ".review")
	entries, err := os.ReadDir(profileDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			continue
		}
		addPath(filepath.Join(profileDir, entry.Name()))
	}

	for _, path := range extraPaths {
		resolved, err := ResolveGuidelinePath(repoRoot, path)
		if err != nil {
			continue
		}
		if isRegularFile(resolved) {
			addPath(resolved)
		}
	}

	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func ResolveGuidelinePath(repoRoot, input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", errors.New("guideline path is empty")
	}
	path := input
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	return filepath.Clean(path), nil
}

func HashGuidelines(paths []string, freeText string) (string, error) {
	paths = append([]string(nil), paths...)
	sort.Strings(paths)
	if len(paths) == 0 && strings.TrimSpace(freeText) == "" {
		return "", nil
	}

	hasher := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = hasher.Write([]byte(path))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write(data)
		_, _ = hasher.Write([]byte{0})
	}

	if strings.TrimSpace(freeText) != "" {
		_, _ = hasher.Write([]byte("free"))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(freeText))
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
