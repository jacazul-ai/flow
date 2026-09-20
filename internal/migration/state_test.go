package migration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jacazul-ai/flow/internal/migration"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
)

func TestLegacyStateMapsFocusAndSessionNote(t *testing.T) {
	dataDir := t.TempDir()
	focus := `{
  "focused_plan": "parity",
  "focused_task_uuid": "11111111-1111-4111-8111-111111111111",
  "task_track": [
    {"uuid": "11111111-1111-4111-8111-111111111111", "plan": "parity"}
  ],
  "plans_of_interest": ["parity"]
}`
	if err := os.WriteFile(filepath.Join(dataDir, "focus.json"), []byte(focus), 0o600); err != nil {
		t.Fatalf("write focus fixture: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dataDir, "session-note-global.md"),
		[]byte("Continue the parity work.\nacknowledged: 2026-08-30T12:00:00Z\n"),
		0o600,
	); err != nil {
		t.Fatalf("write session note fixture: %v", err)
	}

	sessions, err := migration.LoadLegacyState(dataDir)
	if err != nil {
		t.Fatalf("load legacy state: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "global" {
		t.Fatalf("sessions = %#v, want global session", sessions)
	}

	bundle, warnings, err := migration.BuildBundleWithState("project-alpha", []migration.LegacyTask{{
		UUID:        firstUUID,
		Project:     "parity",
		Description: "Task",
		Status:      "pending",
	}}, sessions)
	if err != nil {
		t.Fatalf("build state bundle: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("state warnings = %v, want none", warnings)
	}
	if len(bundle.Sessions) != 1 || bundle.Sessions[0].State.FocusedTaskID != firstUUID {
		t.Fatalf("imported sessions = %#v, want focused task", bundle.Sessions)
	}
	if len(bundle.SessionNotes) != 1 || bundle.SessionNotes[0].AcknowledgedAt == "" {
		t.Fatalf("imported session notes = %#v, want acknowledged note", bundle.SessionNotes)
	}

	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "flow.sqlite3"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer store.Close()
	if _, err := migration.NewImporter(store).Apply(context.Background(), bundle); err != nil {
		t.Fatalf("apply state bundle: %v", err)
	}
	loaded, err := store.LoadFocus(context.Background(), "project-alpha", "global")
	if err != nil {
		t.Fatalf("load imported focus: %v", err)
	}
	if loaded.FocusedTaskID != firstUUID || len(loaded.PlansOfInterest) != 1 {
		t.Fatalf("loaded focus = %#v, want task and interest", loaded)
	}
	note, found, err := store.GetSessionNote(context.Background(), "project-alpha", "global")
	if err != nil || !found || note.AcknowledgedAt == "" {
		t.Fatalf("loaded note = %#v, found=%t, err=%v; want acknowledged note", note, found, err)
	}
}
