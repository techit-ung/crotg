package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeout = 5 * time.Second

type RepoInfo struct {
	RootPath string
}

func DetectRepoRoot(path string) (RepoInfo, error) {
	if strings.TrimSpace(path) == "" {
		return RepoInfo{}, errors.New("path is required")
	}

	output, err := runGit(path, defaultTimeout, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepoInfo{}, err
	}

	return RepoInfo{RootPath: strings.TrimSpace(output)}, nil
}

func ListBranches(repoRoot string) ([]string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, errors.New("repo root is required")
	}

	output, err := runGit(repoRoot, defaultTimeout, "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}

	branches := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		branch := strings.TrimSpace(line)
		if branch == "" || strings.HasSuffix(branch, "/HEAD") {
			continue
		}
		if _, exists := seen[branch]; exists {
			continue
		}
		seen[branch] = struct{}{}
		branches = append(branches, branch)
	}

	return branches, nil
}

func GenerateDiff(repoRoot, baseBranch, branch string) (string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return "", errors.New("repo root is required")
	}
	if strings.TrimSpace(baseBranch) == "" {
		return "", errors.New("base branch is required")
	}
	if strings.TrimSpace(branch) == "" {
		return "", errors.New("branch is required")
	}

	return runGit(repoRoot, defaultTimeout, "diff", "--no-color", "--unified=3", baseBranch+"..."+branch)
}

func runGit(repoRoot string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := exec.CommandContext(ctx, "git", append([]string{"-C", repoRoot}, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}

	return stdout.String(), nil
}
