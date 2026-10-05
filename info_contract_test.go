package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	flow "github.com/jacazul-ai/flow"
)

func TestInfoReportsRuntimeWithoutOpeningStorage(t *testing.T) {
	home := filepath.Join(t.TempDir(), "runtime")
	database := filepath.Join(t.TempDir(), "native", "flow.sqlite3")
	env := flow.Env{
		ProjectID:    "project-alpha",
		SessionID:    "session-alpha",
		DatabasePath: database,
		Home:         home,
	}

	code, stdout, stderr := run(t, context.Background(), env, "info")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	for _, expected := range []string{
		"project_id: project-alpha",
		"project_id_source: runtime",
		"session_id: session-alpha",
		"session_id_source: runtime",
		"home_exists: false",
		"database_path: " + database,
		"database_exists: false",
		"runtime_format: text",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("stdout = %q, want %q", stdout, expected)
		}
	}
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		t.Fatalf("info created database path: %v", err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("info created home path: %v", err)
	}
}

func TestInfoFlagsOverrideRuntimeAndRenderJSON(t *testing.T) {
	runtimeHome := filepath.Join(t.TempDir(), "runtime")
	runtimeDatabase := filepath.Join(t.TempDir(), "runtime.sqlite3")
	flagHome := filepath.Join(t.TempDir(), "flag")
	flagDatabase := filepath.Join(t.TempDir(), "flag.sqlite3")
	flagTaskData := filepath.Join(t.TempDir(), "taskdata")
	env := flow.Env{
		ProjectID:    "runtime-project",
		SessionID:    "runtime-session",
		DatabasePath: runtimeDatabase,
		Home:         runtimeHome,
	}

	code, stdout, stderr := run(
		t,
		context.Background(),
		env,
		"--project", "flag-project",
		"--session", "flag-session",
		"--home", flagHome,
		"--database-path", flagDatabase,
		"--taskdata", flagTaskData,
		"info", "--format", "json",
	)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	for _, expected := range []string{
		`"project_id":"flag-project"`,
		`"project_id_source":"flag"`,
		`"session_id":"flag-session"`,
		`"session_id_source":"flag"`,
		`"home":"` + flagHome + `"`,
		`"home_source":"flag"`,
		`"taskdata":"` + flagTaskData + `"`,
		`"taskdata_source":"flag"`,
		`"database_path":"` + flagDatabase + `"`,
		`"database_source":"flag"`,
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("stdout = %q, want %q", stdout, expected)
		}
	}
	if _, err := os.Stat(flagDatabase); !os.IsNotExist(err) {
		t.Fatalf("info created flagged database path: %v", err)
	}
	if _, err := os.Stat(flagHome); !os.IsNotExist(err) {
		t.Fatalf("info created flagged home path: %v", err)
	}
}

func TestInfoUsesEnvironmentSources(t *testing.T) {
	home := filepath.Join(t.TempDir(), "environment")
	database := filepath.Join(t.TempDir(), "environment.sqlite3")
	t.Setenv("JACAZUL_PROJECT", "environment-project")
	t.Setenv("JACAZUL_SESSION", "environment-session")
	t.Setenv("JACAZUL_HOME", home)
	t.Setenv("JACAZUL_FLOW_DATABASE_PATH", database)
	t.Setenv("JACAZUL_FLOW_FORMAT", "json")

	code, stdout, stderr := run(t, context.Background(), flow.EnvFromOS(), "info")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	for _, expected := range []string{
		`"project_id":"environment-project"`,
		`"project_id_source":"environment"`,
		`"session_id":"environment-session"`,
		`"session_id_source":"environment"`,
		`"home_source":"environment"`,
		`"database_source":"environment"`,
		`"runtime_format":"json"`,
		`"runtime_format_source":"environment"`,
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("stdout = %q, want %q", stdout, expected)
		}
	}
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		t.Fatalf("info created environment database path: %v", err)
	}
}
