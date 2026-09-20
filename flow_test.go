package flow_test

import (
	"bytes"
	"context"
	"io"
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

// captureProcessStdout swaps the process stdout for a pipe while fn runs and
// returns whatever was written there. Nothing should be: Run owns no stream it
// was not handed.
func captureProcessStdout(t *testing.T, fn func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()

	fn()

	os.Stdout = original
	if err := writer.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	leaked, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close pipe reader: %v", err)
	}
	return string(leaked)
}

func TestRunWritesCommandOutputOnlyToProvidedStreams(t *testing.T) {
	env := flow.Env{ProjectID: "project-alpha", Home: t.TempDir()}

	steps := []struct {
		name string
		args []string
		want string
	}{
		{name: "plan", args: []string{"plan", "alpha", "Alpha task"}, want: "Created task"},
		{name: "status", args: []string{"status"}, want: "Alpha task"},
		{name: "next", args: []string{"next"}, want: "Alpha task"},
		{name: "tree", args: []string{"tree"}, want: "Alpha task"},
		{name: "plans", args: []string{"plans"}, want: "alpha"},
		{name: "ponder", args: []string{"ponder"}, want: "alpha"},
		{name: "focus", args: []string{"focus"}, want: "FOCUS"},
		{name: "history", args: []string{"history", "initiative", "alpha"}, want: "HISTORY:"},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			var code int
			var stdout, stderr string
			leaked := captureProcessStdout(t, func() {
				code, stdout, stderr = run(t, context.Background(), env, step.args...)
			})
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr)
			}
			if !strings.Contains(stdout, step.want) {
				t.Fatalf("injected stdout = %q, want %q", stdout, step.want)
			}
			if leaked != "" {
				t.Fatalf("process stdout received %q; Run must write only to the injected streams", leaked)
			}
		})
	}
}

// A command that ran and failed already explains its own next step. Repeating
// a syntax ACTION and dumping the usage buries that guidance under advice that
// does not apply.
func TestRunSeparatesCommandFailuresFromSyntaxErrors(t *testing.T) {
	t.Run("command failure keeps only its own guidance", func(t *testing.T) {
		code, _, stderr := run(t, context.Background(), flow.Env{ProjectID: "project-alpha"}, "status")
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(stderr, "runtime home is required") {
			t.Fatalf("stderr = %q, want the command's own failure", stderr)
		}
		if strings.Contains(stderr, "Review the command syntax") {
			t.Fatalf("stderr = %q, want no syntax guidance for a runtime failure", stderr)
		}
		if strings.Contains(stderr, "Application Options:") {
			t.Fatalf("stderr = %q, want no usage dump for a runtime failure", stderr)
		}
	})

	t.Run("syntax error still shows usage", func(t *testing.T) {
		code, _, stderr := run(t, context.Background(), flow.Env{Home: t.TempDir()}, "no-such-command")
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(stderr, "Review the command syntax") {
			t.Fatalf("stderr = %q, want syntax guidance", stderr)
		}
		if !strings.Contains(stderr, "Application Options:") {
			t.Fatalf("stderr = %q, want the usage dump", stderr)
		}
	})
}
