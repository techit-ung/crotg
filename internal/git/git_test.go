package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectRepoRoot_whenGitRepo_shouldReturnRoot(t *testing.T) {
	// arrange
	repoRoot := initTestRepo(t)

	// act
	repoInfo, err := DetectRepoRoot(repoRoot)

	// assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repoInfo.RootPath != repoRoot {
		t.Fatalf("expected root %q, got %q", repoRoot, repoInfo.RootPath)
	}
}

func TestListBranches_whenBranchesExist_shouldReturnBranches(t *testing.T) {
	// arrange
	repoRoot := initTestRepo(t)
	runGitCommand(t, repoRoot, "branch", "feature/test-branch")

	// act
	branches, err := ListBranches(repoRoot)

	// assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !contains(branches, "feature/test-branch") {
		t.Fatalf("expected branch list to include feature/test-branch, got %v", branches)
	}
}

func TestGenerateDiff_whenBranchHasChanges_shouldReturnDiff(t *testing.T) {
	// arrange
	repoRoot := initTestRepo(t)
	runGitCommand(t, repoRoot, "checkout", "-b", "feature/change")
	writeFile(t, filepath.Join(repoRoot, "example.txt"), "hello\n")
	runGitCommand(t, repoRoot, "add", "example.txt")
	runGitCommand(t, repoRoot, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "add example")

	// act
	diff, err := GenerateDiff(repoRoot, "master", "feature/change")

	// assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if strings.TrimSpace(diff) == "" {
		t.Fatalf("expected diff content, got empty string")
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	runGitCommand(t, tempDir, "init", "-b", "master")
	runGitCommand(t, tempDir, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--allow-empty", "-m", "init")

	return tempDir
}

func runGitCommand(t *testing.T, repoRoot string, args ...string) {
	t.Helper()

	command := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
