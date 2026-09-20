package testharness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Harness owns isolated filesystem and environment state for a contract test.
type Harness struct {
	Root         string
	ProjectID    string
	SessionID    string
	TaskData     string
	CacheDir     string
	BinDir       string
	DatabasePath string
	Environment  []string
}

// NewHarness creates isolated project, session, cache, and executable state.
func NewHarness(t *testing.T, projectID string, sessionID string) *Harness {
	t.Helper()

	root := t.TempDir()
	harness := &Harness{
		Root:         root,
		ProjectID:    projectID,
		SessionID:    sessionID,
		TaskData:     filepath.Join(root, "taskdata"),
		CacheDir:     filepath.Join(root, "cache"),
		BinDir:       filepath.Join(root, "bin"),
		DatabasePath: filepath.Join(root, "database", "jaflow.sqlite3"),
	}

	for _, path := range []string{
		harness.TaskData,
		harness.CacheDir,
		harness.BinDir,
		filepath.Dir(harness.DatabasePath),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create isolated harness directory %q: %v", path, err)
		}
	}

	t.Setenv("HOME", root)
	t.Setenv("PROJECT_ID", projectID)
	t.Setenv("TASKDATA", harness.TaskData)
	t.Setenv("JAFLOW_CACHE_DIR", harness.CacheDir)
	t.Setenv("JACAZUL_FLOW_DATABASE_PATH", harness.DatabasePath)
	t.Setenv("JACAZUL_SESSION_ID", sessionID)
	t.Setenv("JACAZUL_HOME", filepath.Join(root, ".jacazul-ai"))

	harness.Environment = os.Environ()
	for _, entry := range []string{
		"PROJECT_ID=" + projectID,
		"TASKDATA=" + harness.TaskData,
		"JAFLOW_CACHE_DIR=" + harness.CacheDir,
		"JACAZUL_FLOW_DATABASE_PATH=" + harness.DatabasePath,
		"JACAZUL_SESSION_ID=" + sessionID,
		"JACAZUL_HOME=" + filepath.Join(root, ".jacazul-ai"),
	} {
		key, _, _ := strings.Cut(entry, "=")
		harness.Environment = replaceEnvironmentValue(harness.Environment, key, entry)
	}

	return harness
}

// WriteFile writes a file inside the harness and creates its parent directory.
func (h *Harness) WriteFile(t *testing.T, relativePath string, content []byte) string {
	t.Helper()

	path, err := h.ResolvePath(relativePath)
	if err != nil {
		t.Fatalf("resolve isolated file %q: %v", relativePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent directory for %q: %v", relativePath, err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write isolated file %q: %v", relativePath, err)
	}
	return path
}

// ResolvePath validates and resolves a path within the harness root.
func (h *Harness) ResolvePath(relativePath string) (string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return "", fmt.Errorf("relative path is required")
	}
	path := filepath.Join(h.Root, filepath.Clean(relativePath))
	relative, err := filepath.Rel(h.Root, path)
	if err != nil {
		return "", fmt.Errorf("compare harness path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes harness root")
	}
	return path, nil
}

// FakeCommand installs a deterministic executable at the front of PATH.
func (h *Harness) FakeCommand(t *testing.T, name string, script string) string {
	t.Helper()

	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		t.Fatalf("fake command name %q must be a simple executable name", name)
	}
	path := filepath.Join(h.BinDir, name)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake command %q: %v", name, err)
	}

	oldPath := os.Getenv("PATH")
	newPath := h.BinDir + string(os.PathListSeparator) + oldPath
	t.Setenv("PATH", newPath)
	h.Environment = replaceEnvironmentValue(h.Environment, "PATH", "PATH="+newPath)
	return path
}

func replaceEnvironmentValue(environment []string, key string, value string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, value)
}

// RunCommand executes a command with the harness environment and captures all
// output for assertions.
func (h *Harness) RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = h.Environment
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("run %s: %w: %s", name, err, output)
	}
	return string(output), nil
}
