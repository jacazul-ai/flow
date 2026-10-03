package config

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProjectIDUsesCanonicalDirectoryForNonGitPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "parent", "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create project directory: %v", err)
	}

	if got, want := ResolveProjectID(root), "parent_project"; got != want {
		t.Fatalf("project ID = %q, want %q", got, want)
	}
}

func TestResolveProjectIDUsesGitTopLevelForNormalRepository(t *testing.T) {
	requireGit(t)
	root := filepath.Join(t.TempDir(), "parent", "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create repository directory: %v", err)
	}
	runGit(t, root, "init")
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	if got, want := ResolveProjectID(nested), "parent_project"; got != want {
		t.Fatalf("project ID = %q, want %q", got, want)
	}
}

func TestResolveProjectIDUsesSharedRootForBareLinkedWorktree(t *testing.T) {
	requireGit(t)
	projectRoot := filepath.Join(t.TempDir(), "jacazul-ai", "flow")
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create source repository: %v", err)
	}
	runGit(t, source, "init")
	runGit(t, source, "config", "user.name", "Flow Test")
	runGit(t, source, "config", "user.email", "flow-test@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "README"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	runGit(t, source, "add", "README")
	runGit(t, source, "commit", "-m", "seed")
	branch := runGit(t, source, "branch", "--show-current")

	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("create shared project root: %v", err)
	}
	bare := filepath.Join(projectRoot, ".bare")
	runGitCommand(t, "clone", "--bare", source, bare)
	worktree := filepath.Join(projectRoot, "master")
	runGitCommand(t, "--git-dir", bare, "worktree", "add", worktree, branch)

	if got, want := ResolveProjectID(worktree), "jacazul-ai_flow"; got != want {
		t.Fatalf("worktree project ID = %q, want %q", got, want)
	}
	if got, want := ResolveProjectID(projectRoot), "jacazul-ai_flow"; got != want {
		t.Fatalf("shared-root project ID = %q, want %q", got, want)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is required for this contract test: %v", err)
	}
}

func runGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	return runGitCommand(t, append([]string{"-C", directory}, args...)...)
}

func runGitCommand(t *testing.T, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(string(output))
}
