package flow_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	flow "github.com/jacazul-ai/flow"
)

func run(t *testing.T, ctx context.Context, env flow.Env, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := flow.Run(ctx, args, env, flow.Streams{Stdout: &stdout, Stderr: &stderr})
	return code, stdout.String(), stderr.String()
}

func TestRunWritesHelpToProvidedStreams(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "explicit help", args: []string{"help"}},
		{name: "no arguments", args: nil},
		{name: "help flag", args: []string{"--help"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := run(t, context.Background(), flow.Env{Home: t.TempDir()}, test.args...)
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr)
			}
			if !strings.Contains(stdout, "ROLE") {
				t.Fatalf("stdout = %q, want root help", stdout)
			}
		})
	}
}

func TestRunReportsUnknownCommandOnStderr(t *testing.T) {
	code, stdout, stderr := run(t, context.Background(), flow.Env{Home: t.TempDir()}, "no-such-command")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "ERROR:") || !strings.Contains(stderr, "ACTION:") {
		t.Fatalf("stderr = %q, want ERROR and ACTION guidance", stderr)
	}
}

func TestRunPrintsVersion(t *testing.T) {
	code, stdout, stderr := run(t, context.Background(), flow.Env{Home: t.TempDir()}, "--version", "help")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" || strings.Contains(stdout, "ROLE") {
		t.Fatalf("stdout = %q, want only a version", stdout)
	}
}

func TestRunStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	code, _, stderr := run(t, ctx, flow.Env{Home: t.TempDir()}, "help")
	if code != 1 || !strings.Contains(stderr, "context canceled") {
		t.Fatalf("exit = %d, stderr = %q, want cancellation failure", code, stderr)
	}
}

func TestRunRequiresHomeForDefaultPaths(t *testing.T) {
	code, _, stderr := run(t, context.Background(), flow.Env{ProjectID: "project-alpha"}, "plan", "alpha", "Alpha task")
	if code != 1 || !strings.Contains(stderr, "runtime home is required") {
		t.Fatalf("exit = %d, stderr = %q, want missing home failure", code, stderr)
	}
}

func TestRunUsesEnvInsteadOfProcessEnvironment(t *testing.T) {
	poisoned := filepath.Join(t.TempDir(), "from-process-env.sqlite3")
	t.Setenv("JACAZUL_FLOW_DATABASE_PATH", poisoned)
	t.Setenv("PROJECT_ID", "from-process-env")

	database := filepath.Join(t.TempDir(), "from-env.sqlite3")
	env := flow.Env{ProjectID: "project-alpha", DatabasePath: database, Home: t.TempDir()}

	code, _, stderr := run(t, context.Background(), env, "plan", "alpha", "Alpha task")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr)
	}
	if _, err := os.Stat(database); err != nil {
		t.Fatalf("Env database was not used: %v", err)
	}
	if _, err := os.Stat(poisoned); !os.IsNotExist(err) {
		t.Fatalf("process environment database was touched: %v", err)
	}
}
