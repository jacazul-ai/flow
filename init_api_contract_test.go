package flow_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	flow "github.com/jacazul-ai/flow"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
)

func TestInitAPICreatesMissingDatabase(t *testing.T) {
	env := initEnv(t, "init-project")

	result, err := flow.Init(context.Background(), env)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	want := flow.InitResult{Path: env.DatabasePath, Created: true, From: 0, To: 15}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
	if _, err := os.Stat(env.DatabasePath); err != nil {
		t.Fatalf("Init did not create the database: %v", err)
	}
}

func TestInitAPIReportsCurrentDatabase(t *testing.T) {
	env := initEnv(t, "init-project")
	if _, err := flow.Init(context.Background(), env); err != nil {
		t.Fatalf("first Init: %v", err)
	}

	result, err := flow.Init(context.Background(), env)
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	want := flow.InitResult{Path: env.DatabasePath, From: 15, To: 15}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
}

func TestInitAPIUpgradesOlderSchema(t *testing.T) {
	env := initEnv(t, "init-project")
	if err := os.MkdirAll(filepath.Dir(env.DatabasePath), 0o700); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	downgradeSchemaForTest(t, env.DatabasePath)

	result, err := flow.Init(context.Background(), env)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	want := flow.InitResult{Path: env.DatabasePath, Upgraded: true, From: 14, To: 15}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
}

func TestInitAPIReportsMoveWithLegacyPath(t *testing.T) {
	home := t.TempDir()
	env := flow.Env{ProjectID: "init-project", SessionID: "init-session", Home: home}
	legacy := filepath.Join(home, "jaflow", "init-project", "jaflow.sqlite3")
	current := filepath.Join(home, "flow", "init-project", "flow.sqlite3")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatalf("create legacy directory: %v", err)
	}
	downgradeSchemaForTest(t, legacy)

	result, err := flow.Init(context.Background(), env)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	want := flow.InitResult{Path: current, LegacyPath: legacy, Moved: true, Upgraded: true, From: 14, To: 15}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
}

func TestInitAPIClassifiesNewerSchema(t *testing.T) {
	env := initEnv(t, "init-project")
	if err := os.MkdirAll(filepath.Dir(env.DatabasePath), 0o700); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	setSchemaVersionForTest(t, env.DatabasePath, 99)

	_, err := flow.Init(context.Background(), env)
	if !errors.Is(err, flow.ErrSchemaNewer) {
		t.Fatalf("err = %v, want ErrSchemaNewer", err)
	}
	if errors.Is(err, flow.ErrDatabaseConflict) {
		t.Fatalf("err = %v, unexpectedly matches ErrDatabaseConflict", err)
	}
	if !strings.Contains(err.Error(), "newer than") || !strings.Contains(err.Error(), "ACTION:") {
		t.Fatalf("err = %q, want the actionable newer schema message", err)
	}
}

func TestInitAPIClassifiesDatabaseConflict(t *testing.T) {
	home := t.TempDir()
	env := flow.Env{ProjectID: "init-project", SessionID: "init-session", Home: home}
	for _, path := range []string{
		filepath.Join(home, "jaflow", "init-project", "jaflow.sqlite3"),
		filepath.Join(home, "flow", "init-project", "flow.sqlite3"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create database directory: %v", err)
		}
		store, err := sqlite.Open(context.Background(), path)
		if err != nil {
			t.Fatalf("create database %s: %v", path, err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("close database %s: %v", path, err)
		}
	}

	_, err := flow.Init(context.Background(), env)
	if !errors.Is(err, flow.ErrDatabaseConflict) {
		t.Fatalf("err = %v, want ErrDatabaseConflict", err)
	}
	if errors.Is(err, flow.ErrSchemaNewer) {
		t.Fatalf("err = %v, unexpectedly matches ErrSchemaNewer", err)
	}
	if !strings.Contains(err.Error(), "both exist") || !strings.Contains(err.Error(), "ACTION:") {
		t.Fatalf("err = %q, want the actionable conflict message", err)
	}

	code, _, stderr := run(t, context.Background(), env, "init")
	if code == 0 || !strings.Contains(stderr, "both exist") || !strings.Contains(stderr, "ACTION:") {
		t.Fatalf("CLI init = %d, stderr = %q; want the same conflict error", code, stderr)
	}
}

func TestInitAPIWritesNoWorkflowState(t *testing.T) {
	env := initEnv(t, "init-project")
	if _, err := flow.Init(context.Background(), env); err != nil {
		t.Fatalf("Init: %v", err)
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
			t.Fatalf("Init wrote %d %s rows, want none", count, table)
		}
	}
}

func TestInitAPIKeepsProjectsIsolated(t *testing.T) {
	home := t.TempDir()
	alpha := flow.Env{ProjectID: "project-alpha", SessionID: "init-session", Home: home}
	beta := flow.Env{ProjectID: "project-beta", SessionID: "init-session", Home: home}
	betaDatabase := filepath.Join(home, "flow", "project-beta", "flow.sqlite3")

	alphaResult, err := flow.Init(context.Background(), alpha)
	if err != nil {
		t.Fatalf("alpha Init: %v", err)
	}
	if _, err := os.Stat(betaDatabase); !os.IsNotExist(err) {
		t.Fatalf("alpha Init created beta database: %v", err)
	}
	betaResult, err := flow.Init(context.Background(), beta)
	if err != nil {
		t.Fatalf("beta Init: %v", err)
	}
	if !betaResult.Created || betaResult.Path != betaDatabase || betaResult.Path == alphaResult.Path {
		t.Fatalf("beta result = %+v, want its own created database", betaResult)
	}
	again, err := flow.Init(context.Background(), alpha)
	if err != nil || again.Created || again.Upgraded {
		t.Fatalf("alpha Init after beta = %+v, %v; want alpha unchanged", again, err)
	}
}

func TestInitAPIHonorsCanceledContext(t *testing.T) {
	env := initEnv(t, "init-project")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := flow.Init(ctx, env); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(env.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("canceled Init created the database: %v", err)
	}
}
