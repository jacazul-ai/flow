package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// seedLegacyDatabase creates a closed database holding one initiative so a
// relocation can be verified by the data that survives it.
func seedLegacyDatabase(t *testing.T, path string) {
	t.Helper()

	store, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := store.GetOrCreateInitiative(context.Background(), task.CreateInitiativeInput{
		ProjectID: "project-alpha",
		Name:      "parity",
	}); err != nil {
		store.Close()
		t.Fatalf("seed legacy initiative: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
}

func legacyPaths(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	return filepath.Join(root, "jaflow", "project-alpha", "jaflow.sqlite3"),
		filepath.Join(root, "flow", "project-alpha", "flow.sqlite3")
}

func TestMoveLegacyDatabaseRelocatesTheDataAndLeavesNothingBehind(t *testing.T) {
	legacy, current := legacyPaths(t)
	seedLegacyDatabase(t, legacy)

	if err := os.WriteFile(legacy+"-wal", []byte("stray"), 0o600); err != nil {
		t.Fatalf("write stray sidecar: %v", err)
	}

	moved, err := sqlite.MoveLegacyDatabase(legacy, current)
	if err != nil {
		t.Fatalf("move legacy database: %v", err)
	}
	if !moved {
		t.Fatal("move legacy database reported no move")
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(legacy + suffix); !os.IsNotExist(err) {
			t.Fatalf("legacy file %s survived the move", legacy+suffix)
		}
	}

	store := openStore(t, current)
	initiatives, err := store.ListInitiatives(context.Background(), "project-alpha", true, true)
	if err != nil {
		t.Fatalf("list initiatives: %v", err)
	}
	if len(initiatives) != 1 {
		t.Fatalf("relocated database holds %d initiatives, want 1", len(initiatives))
	}
}

func TestMoveLegacyDatabaseIgnoresMissingLegacy(t *testing.T) {
	legacy, current := legacyPaths(t)

	moved, err := sqlite.MoveLegacyDatabase(legacy, current)
	if err != nil {
		t.Fatalf("move legacy database: %v", err)
	}
	if moved {
		t.Fatal("move legacy database reported a move without a legacy file")
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatal("current database was created without a legacy file")
	}
}

func TestMoveLegacyDatabaseIgnoresEmptyAndIdenticalPaths(t *testing.T) {
	legacy, current := legacyPaths(t)
	seedLegacyDatabase(t, legacy)

	for name, pair := range map[string][2]string{
		"empty legacy":  {"", current},
		"empty current": {legacy, ""},
		"same path":     {legacy, legacy},
	} {
		moved, err := sqlite.MoveLegacyDatabase(pair[0], pair[1])
		if err != nil {
			t.Fatalf("%s: move legacy database: %v", name, err)
		}
		if moved {
			t.Fatalf("%s: move legacy database reported a move", name)
		}
	}
}

func TestMoveLegacyDatabaseFailsClosedWhenBothExist(t *testing.T) {
	legacy, current := legacyPaths(t)
	seedLegacyDatabase(t, legacy)
	seedLegacyDatabase(t, current)

	moved, err := sqlite.MoveLegacyDatabase(legacy, current)
	if err == nil {
		t.Fatal("move legacy database accepted two databases")
	}
	if moved {
		t.Fatal("move legacy database reported a move after failing")
	}
	if !strings.Contains(err.Error(), "ACTION:") {
		t.Fatalf("error %q does not explain the next action", err)
	}
	for _, path := range []string{legacy, current} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("database %s was disturbed: %v", path, statErr)
		}
	}
}

func TestMoveLegacyDatabaseFailsClosedWhenLegacyIsLocked(t *testing.T) {
	legacy, current := legacyPaths(t)
	seedLegacyDatabase(t, legacy)

	holder, err := sql.Open("sqlite", legacy)
	if err != nil {
		t.Fatalf("open holder connection: %v", err)
	}
	defer holder.Close()

	connection, err := holder.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve holder connection: %v", err)
	}
	defer connection.Close()

	if _, err := connection.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("take write lock: %v", err)
	}
	defer connection.ExecContext(context.Background(), "ROLLBACK")

	moved, err := sqlite.MoveLegacyDatabase(legacy, current)
	if err == nil {
		t.Fatal("move legacy database accepted a locked database")
	}
	if moved {
		t.Fatal("move legacy database reported a move after failing")
	}
	if !strings.Contains(err.Error(), "ACTION:") {
		t.Fatalf("error %q does not explain the next action", err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy database was disturbed: %v", err)
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatal("current database was created while the legacy one was locked")
	}
}
