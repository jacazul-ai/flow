package config_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jacazul-ai/flow/internal/config"
)

func TestLegacyTaskDataDoesNotReplaceNativeDatabase(t *testing.T) {
	home := t.TempDir()
	opts := config.AppOptions{
		ProjectID: "project-alpha",
		TaskData:  filepath.Join(t.TempDir(), "legacy-taskdata"),
		Runtime:   config.Runtime{Home: home},
	}
	if err := config.Resolve(&opts); err != nil {
		t.Fatalf("resolve options: %v", err)
	}
	want := filepath.Join(home, "jaflow", "project-alpha", "jaflow.sqlite3")
	if opts.DatabasePath != want {
		t.Fatalf("database path = %q, want native path %q", opts.DatabasePath, want)
	}
	if opts.TaskData == "" {
		t.Fatal("legacy taskdata context was unexpectedly discarded")
	}
}

func TestResolveIgnoresProcessEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROJECT_ID", "from-env")
	t.Setenv("JACAZUL_SESSION_ID", "session-from-env")
	t.Setenv("JACAZUL_FLOW_DATABASE_PATH", filepath.Join(t.TempDir(), "env.sqlite3"))
	t.Setenv("JACAZUL_HOME", t.TempDir())

	opts := config.AppOptions{Runtime: config.Runtime{
		ProjectID: "from-runtime",
		SessionID: "session-from-runtime",
		Home:      home,
	}}
	if err := config.Resolve(&opts); err != nil {
		t.Fatalf("resolve options: %v", err)
	}
	if opts.ProjectID != "from-runtime" || opts.SessionID != "session-from-runtime" {
		t.Fatalf("identity = %q/%q, want runtime values", opts.ProjectID, opts.SessionID)
	}
	want := filepath.Join(home, "jaflow", "from-runtime", "jaflow.sqlite3")
	if opts.DatabasePath != want {
		t.Fatalf("database path = %q, want %q derived from runtime home", opts.DatabasePath, want)
	}
}

func TestFlagsOverrideRuntimeContext(t *testing.T) {
	flagPath := filepath.Join(t.TempDir(), "flag.sqlite3")
	opts := config.AppOptions{
		ProjectID:    "from-flag",
		DatabasePath: flagPath,
		Runtime: config.Runtime{
			ProjectID:    "from-runtime",
			DatabasePath: filepath.Join(t.TempDir(), "runtime.sqlite3"),
			Home:         t.TempDir(),
		},
	}
	if err := config.Resolve(&opts); err != nil {
		t.Fatalf("resolve options: %v", err)
	}
	if opts.ProjectID != "from-flag" || opts.DatabasePath != flagPath {
		t.Fatalf("options = %q/%q, want flag values", opts.ProjectID, opts.DatabasePath)
	}
}

func TestResolveRequiresHomeForDefaultPaths(t *testing.T) {
	opts := config.AppOptions{Runtime: config.Runtime{ProjectID: "project-alpha"}}
	err := config.Resolve(&opts)
	if !errors.Is(err, config.ErrHomeRequired) {
		t.Fatalf("resolve error = %v, want ErrHomeRequired", err)
	}
}
