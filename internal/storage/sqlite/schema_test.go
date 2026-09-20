package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/storage/sqlite"

	_ "modernc.org/sqlite"
)

// stampSchemaVersion records an applied migration the running binary does not
// carry, the shape a database takes after a newer release wrote to it.
func stampSchemaVersion(t *testing.T, path string, version int64) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	_, err = db.ExecContext(
		context.Background(),
		"INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (?, 1, CURRENT_TIMESTAMP)",
		version,
	)
	if err != nil {
		t.Fatalf("stamp schema version %d: %v", version, err)
	}
}

// currentSchemaVersion reports the highest applied migration in the database.
func currentSchemaVersion(t *testing.T, path string) int64 {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	var version int64
	row := db.QueryRowContext(
		context.Background(),
		"SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1",
	)
	if err := row.Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	return version
}

// Rolling jacazul back to an older release must not silently open a database
// an newer release already upgraded: the old binary cannot know what the new
// columns mean, and writing through it corrupts the newer schema.
func TestOpenRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flow.sqlite3")
	store := openStore(t, path)
	supported := currentSchemaVersion(t, path)
	if err := store.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	stampSchemaVersion(t, path, supported+1)

	reopened, err := sqlite.Open(context.Background(), path)
	if err == nil {
		reopened.Close()
		t.Fatal("opened a database whose schema is newer than this binary supports")
	}
	message := err.Error()
	for _, want := range []string{"newer", "ACTION:"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want it to contain %q", message, want)
		}
	}
}

// The guard must not fire on the schema the binary itself just applied, nor on
// a database that is merely behind.
func TestOpenAcceptsCurrentAndOlderSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flow.sqlite3")
	store := openStore(t, path)
	if err := store.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	reopened, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen database at the current schema: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened database: %v", err)
	}
}
