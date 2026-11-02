package flow_test

import (
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

func countSessionRows(t *testing.T, path string, table string, sessionID string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE session_id = ?", sessionID).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestFocusBackKeepsSessionNotesAndDumps(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")
	output, err := runFlow(t, harness, "plan", "focus-back", "First")
	if err != nil {
		t.Fatalf("create plan: %v\n%s", err, output)
	}
	taskID := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(taskID) != 2 {
		t.Fatalf("plan output = %q, want a task UUID", output)
	}
	if output, err := runFlow(t, harness, "focus", "ind", "task", taskID[1]); err != nil {
		t.Fatalf("focus ind task: %v\n%s", err, output)
	}
	output, err = runFlow(t, harness, "session", "dump")
	if err != nil {
		t.Fatalf("session dump: %v\n%s", err, output)
	}
	dumpID := regexp.MustCompile(`Session dump draft ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(dumpID) != 2 {
		t.Fatalf("session dump = %q, want draft UUID", output)
	}
	if output, err := runFlow(t, harness, "session", "finalize", dumpID[1], "--actions", "Resume the first task"); err != nil {
		t.Fatalf("finalize dump: %v\n%s", err, output)
	}
	db, err := sql.Open("sqlite", harness.DatabasePath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO session_notes (project_id, session_id, content, acknowledged_at, updated_at)
		VALUES ('project', 'session', 'Legacy handoff note', '', '2026-10-31T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed legacy session note: %v", err)
	}
	db.Close()

	if output, err := runFlow(t, harness, "focus", "back"); err != nil || !strings.Contains(output, "Switched back to global focus") {
		t.Fatalf("focus back = %q, err %v; want global fallback", output, err)
	}

	if got := countSessionRows(t, harness.DatabasePath, "session_notes", "session"); got != 1 {
		t.Fatalf("session notes after focus back = %d, want the note kept", got)
	}
	if got := countSessionRows(t, harness.DatabasePath, "session_dumps", "session"); got != 1 {
		t.Fatalf("session dumps after focus back = %d, want the dump kept", got)
	}
	if got := countSessionRows(t, harness.DatabasePath, "sessions", "session"); got != 0 {
		t.Fatalf("session focus rows after focus back = %d, want the independent focus gone", got)
	}
	if got := countSessionRows(t, harness.DatabasePath, "cache_entries", "session"); got != 0 {
		t.Fatalf("session cache rows after focus back = %d, want the derived cache cleared", got)
	}
	output, err = runFlow(t, harness, "session", "resume")
	if err != nil || !strings.Contains(output, "SESSION HANDOFF") || !strings.Contains(output, "Resume the first task") {
		t.Fatalf("session resume after focus back = %q, err %v; want the pending dump", output, err)
	}
}
