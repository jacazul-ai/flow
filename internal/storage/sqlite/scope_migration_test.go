package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jacazul-ai/flow/internal/storage/sqlite"
)

// Before v17 every sessions row meant an independent focus, so the upgrade
// marks existing rows and dumps 'ind' and only the project's 'global' session
// 'global'.
func TestSessionScopeMigrationBackfillsExistingRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "flow.sqlite3")
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	for _, statement := range []string{
		"DROP INDEX session_dumps_lookup_idx",
		"CREATE INDEX session_dumps_lookup_idx ON session_dumps (project_id, session_id, sequence)",
		"ALTER TABLE session_dumps DROP COLUMN scope",
		"ALTER TABLE sessions DROP COLUMN scope",
		"DELETE FROM goose_db_version WHERE version_id = 17",
		`INSERT INTO sessions (project_id, session_id, task_stack_json, updated_at)
		 VALUES ('project', 'global', '[]', 'now'), ('project', 'agent', '[]', 'now')`,
		`INSERT INTO session_dumps (id, project_id, session_id, sequence, status,
		  created_at, content_encoding, content_blob, content_size, content_sha256, updated_at)
		 VALUES ('g', 'project', 'global', 1, 'read', 'now', 'gzip', x'', 0, '', 'now'),
		        ('a', 'project', 'agent', 1, 'read', 'now', 'gzip', x'', 0, '', 'now')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("shape v16 database with %q: %v", statement, err)
		}
	}
	db.Close()

	openStore(t, path)
	if version := currentSchemaVersion(t, path); version != 17 {
		t.Fatalf("schema version = %d, want 17", version)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer db.Close()
	for _, check := range []struct{ table, session, want string }{
		{"sessions", "global", "global"},
		{"sessions", "agent", "ind"},
		{"session_dumps", "global", "global"},
		{"session_dumps", "agent", "ind"},
	} {
		var scope string
		if err := db.QueryRow("SELECT scope FROM "+check.table+" WHERE session_id = ?", check.session).Scan(&scope); err != nil {
			t.Fatalf("read %s scope of %s: %v", check.table, check.session, err)
		}
		if scope != check.want {
			t.Fatalf("%s scope of %s = %q, want %q", check.table, check.session, scope, check.want)
		}
	}
}
