package flow_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	flow "github.com/jacazul-ai/flow"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
)

func initEnv(t *testing.T, projectID string) flow.Env {
	t.Helper()
	return flow.Env{
		ProjectID:    projectID,
		SessionID:    "init-session",
		DatabasePath: filepath.Join(t.TempDir(), "database", "flow.sqlite3"),
		Home:         filepath.Join(t.TempDir(), "home"),
	}
}

func TestInitCreatesMissingDatabaseAndReportsIt(t *testing.T) {
	env := initEnv(t, "init-project")

	code, stdout, stderr := run(t, context.Background(), env, "init")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	want := "Created project database at " + env.DatabasePath + " (schema v15)\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if _, err := os.Stat(env.DatabasePath); err != nil {
		t.Fatalf("init did not create the database: %v", err)
	}
}

func TestInitIsQuietWhenDatabaseIsCurrent(t *testing.T) {
	env := initEnv(t, "init-project")
	if code, _, stderr := run(t, context.Background(), env, "init"); code != 0 {
		t.Fatalf("first init exit = %d, stderr = %q", code, stderr)
	}

	code, stdout, stderr := run(t, context.Background(), env, "init")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("second init = %d, stdout = %q, stderr = %q; want quiet success", code, stdout, stderr)
	}

	code, stdout, stderr = run(t, context.Background(), env, "init", "--verbose")
	if code != 0 {
		t.Fatalf("verbose init exit = %d, stderr = %q", code, stderr)
	}
	want := "Project database is current at " + env.DatabasePath + " (schema v15)\n"
	if stdout != want || stderr != "" {
		t.Fatalf("verbose init stdout = %q, stderr = %q; want %q", stdout, stderr, want)
	}
}

func TestInitUpgradesOlderSchemaAndReportsTransition(t *testing.T) {
	env := initEnv(t, "init-project")
	if err := os.MkdirAll(filepath.Dir(env.DatabasePath), 0o700); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	downgradeSchemaForTest(t, env.DatabasePath)

	code, stdout, stderr := run(t, context.Background(), env, "init")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "Schema upgrade required: v14 -> v15") {
		t.Fatalf("stderr = %q, want upgrade notice", stderr)
	}
	want := "Upgraded project database at " + env.DatabasePath + " (v14 -> v15)\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestInitRejectsNewerSchemaWithAction(t *testing.T) {
	env := initEnv(t, "init-project")
	if err := os.MkdirAll(filepath.Dir(env.DatabasePath), 0o700); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	setSchemaVersionForTest(t, env.DatabasePath, 99)

	code, stdout, stderr := run(t, context.Background(), env, "init")
	if code == 0 {
		t.Fatalf("exit = 0, stdout = %q; want newer schema rejected", stdout)
	}
	if !strings.Contains(stderr, "newer than") || !strings.Contains(stderr, "ACTION:") {
		t.Fatalf("stderr = %q, want newer schema error with ACTION", stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want no state change report", stdout)
	}
}

func TestInitMovesLegacyDatabaseBeforeReporting(t *testing.T) {
	home := t.TempDir()
	env := flow.Env{ProjectID: "init-project", SessionID: "init-session", Home: home}
	legacy := filepath.Join(home, "jaflow", "init-project", "jaflow.sqlite3")
	current := filepath.Join(home, "flow", "init-project", "flow.sqlite3")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatalf("create legacy directory: %v", err)
	}
	store, err := sqlite.Open(context.Background(), legacy)
	if err != nil {
		t.Fatalf("create legacy database: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	code, stdout, stderr := run(t, context.Background(), env, "init")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "Moved legacy database to "+current+"\n" {
		t.Fatalf("stdout = %q, want only the move report", stdout)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy database still present: %v", err)
	}
}

func TestInitTouchesNoSessionFocusOrCacheState(t *testing.T) {
	env := initEnv(t, "init-project")
	if code, _, stderr := run(t, context.Background(), env, "init"); code != 0 {
		t.Fatalf("init exit = %d, stderr = %q", code, stderr)
	}

	db, err := sql.Open("sqlite", env.DatabasePath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	for _, table := range []string{"initiatives", "tasks", "sessions", "session_notes", "session_dumps", "cache_entries"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("init wrote %d %s rows, want none", count, table)
		}
	}
}

func TestInitKeepsProjectsIsolated(t *testing.T) {
	home := t.TempDir()
	alpha := flow.Env{ProjectID: "project-alpha", SessionID: "init-session", Home: home}
	beta := flow.Env{ProjectID: "project-beta", SessionID: "init-session", Home: home}
	alphaDatabase := filepath.Join(home, "flow", "project-alpha", "flow.sqlite3")
	betaDatabase := filepath.Join(home, "flow", "project-beta", "flow.sqlite3")

	if code, _, stderr := run(t, context.Background(), alpha, "init"); code != 0 {
		t.Fatalf("alpha init exit = %d, stderr = %q", code, stderr)
	}
	if _, err := os.Stat(betaDatabase); !os.IsNotExist(err) {
		t.Fatalf("alpha init created beta database: %v", err)
	}

	code, stdout, stderr := run(t, context.Background(), beta, "init")
	if code != 0 {
		t.Fatalf("beta init exit = %d, stderr = %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "Created project database at "+betaDatabase) {
		t.Fatalf("beta init stdout = %q, want beta database created", stdout)
	}
	code, stdout, stderr = run(t, context.Background(), alpha, "init", "--verbose")
	if code != 0 || !strings.HasPrefix(stdout, "Project database is current at "+alphaDatabase) {
		t.Fatalf("alpha verbose init = %d, stdout = %q, stderr = %q; want alpha unchanged", code, stdout, stderr)
	}
}

func TestInitHonorsDatabasePathFlag(t *testing.T) {
	env := initEnv(t, "init-project")
	explicit := filepath.Join(t.TempDir(), "explicit", "flow.sqlite3")

	code, stdout, stderr := run(t, context.Background(), env, "--database-path", explicit, "init")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "Created project database at "+explicit+" (schema v15)\n" {
		t.Fatalf("stdout = %q, want explicit path created", stdout)
	}
	if _, err := os.Stat(env.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("init created the runtime database despite the flag: %v", err)
	}
}

func TestInitRejectsArgumentsWithAction(t *testing.T) {
	env := initEnv(t, "init-project")

	code, _, stderr := run(t, context.Background(), env, "init", "extra")
	if code == 0 || !strings.Contains(stderr, "init accepts no arguments") || !strings.Contains(stderr, "ACTION:") {
		t.Fatalf("exit = %d, stderr = %q; want argument error with ACTION", code, stderr)
	}
	if _, err := os.Stat(env.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("rejected init created the database: %v", err)
	}
}
