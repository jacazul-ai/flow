package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

func TestMigrationDryRunDoesNotCreateTargetDatabase(t *testing.T) {
	binary := buildFlow(t)
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "id": 1,
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "parity",
    "description": "[EXECUTE] Import task",
    "status": "pending"
  }
]`))

	output, err := runFlow(t, binary, harness, "migrate", "taskwarrior", "--source", source)
	if err != nil {
		t.Fatalf("migration dry-run: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Migration dry-run: no changes written") {
		t.Fatalf("dry-run output = %q, want no-write confirmation", output)
	}
	if _, err := os.Stat(harness.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("dry-run created target database: %v", err)
	}
}

func TestMigrationApplyIsRepeatableAndPreservesNativeState(t *testing.T) {
	binary := buildFlow(t)
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "id": 1,
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "parity",
    "description": "[EXECUTE] Import task",
    "status": "pending",
    "externalid": "#JAF-123",
    "annotations": [
      {"entry": "20260830T120000Z", "description": "DECISION: Keep UUID"}
    ]
  }
]`))

	for _, args := range [][]string{
		{"migrate", "taskwarrior", "--source", source, "--apply"},
		{"migrate", "taskwarrior", "--source", source, "--apply"},
	} {
		output, err := runFlow(t, binary, harness, args...)
		if err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
		if !strings.Contains(output, "Migration applied") {
			t.Fatalf("apply output = %q, want confirmation", output)
		}
	}

	output, err := runFlow(t, binary, harness, "status", "parity", "--force")
	if err != nil {
		t.Fatalf("read imported state: %v\n%s", err, output)
	}
	for _, expected := range []string{"Import task", "#JAF-123"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("imported status = %q, want %s", output, expected)
		}
	}
	output, err = runFlow(t, binary, harness, "notes", "11111111-1111-4111-8111-111111111111")
	if err != nil || !strings.Contains(output, "DECISION: Keep UUID") {
		t.Fatalf("imported notes = %q, err %v; want one decision", output, err)
	}
}

func TestMigrationIsolatedByProject(t *testing.T) {
	binary := buildFlow(t)
	first := testharness.NewHarness(t, "project-alpha", "session")
	second := testharness.NewHarness(t, "project-beta", "session")
	source := first.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "private",
    "description": "Alpha imported task",
    "status": "pending"
  }
]`))

	output, err := runFlow(t, binary, first, "migrate", "taskwarrior", "--source", source, "--apply")
	if err != nil {
		t.Fatalf("apply alpha migration: %v\n%s", err, output)
	}
	output, err = runFlow(t, binary, first, "status", "private", "--force")
	if err != nil || !strings.Contains(output, "Alpha imported task") {
		t.Fatalf("alpha status = %q, err %v; want imported task", output, err)
	}
	output, err = runFlow(t, binary, second, "status", "--force")
	if err != nil {
		t.Fatalf("read beta status: %v\n%s", err, output)
	}
	if strings.Contains(output, "Alpha imported task") {
		t.Fatalf("beta observed alpha migration: %q", output)
	}
}

func TestMigrationTransfersFocusAndSessionNote(t *testing.T) {
	binary := buildFlow(t)
	harness := testharness.NewHarness(t, "project-alpha", "global")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "parity",
    "description": "Imported focus task",
    "status": "pending"
  }
]`))
	legacyDir := filepath.Join(harness.Root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("create legacy state directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "focus.json"), []byte(`{
  "focused_plan": "parity",
  "focused_task_uuid": "11111111-1111-4111-8111-111111111111",
  "task_track": [{"uuid": "11111111-1111-4111-8111-111111111111", "plan": "parity"}],
  "plans_of_interest": ["parity"]
}`), 0o600); err != nil {
		t.Fatalf("write legacy focus: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "session-note-global.md"), []byte("Resume imported task.\n"), 0o600); err != nil {
		t.Fatalf("write legacy session note: %v", err)
	}

	output, err := runFlow(t, binary, harness, "migrate", "taskwarrior", "--source", source, "--legacy-data-dir", legacyDir, "--apply")
	if err != nil {
		t.Fatalf("apply focus migration: %v\n%s", err, output)
	}
	output, err = runFlow(t, binary, harness, "focus")
	if err != nil || !strings.Contains(output, "Initiative: parity") || !strings.Contains(output, "Task: 11111111") {
		t.Fatalf("migrated focus = %q, err %v; want parity anchor", output, err)
	}
	output, err = runFlow(t, binary, harness, "session", "resume")
	if err != nil || !strings.Contains(output, "Resume imported task.") {
		t.Fatalf("migrated session note = %q, err %v; want handoff note", output, err)
	}
}

func TestMigrationRejectsConflictingModes(t *testing.T) {
	binary := buildFlow(t)
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte("[]"))

	output, err := runFlow(t, binary, harness, "migrate", "taskwarrior", "--source", source, "--apply", "--dry-run")
	if err == nil || !strings.Contains(output, "cannot be combined") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("conflicting migration flags = %q, err %v; want actionable error", output, err)
	}
}
