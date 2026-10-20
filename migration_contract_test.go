package flow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

func TestMigrationDryRunDoesNotCreateTargetDatabase(t *testing.T) {
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

	output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source)
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
		output, err := runFlow(t, harness, args...)
		if err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
		if !strings.Contains(output, "Migration applied") {
			t.Fatalf("apply output = %q, want confirmation", output)
		}
	}

	output, err := runFlow(t, harness, "status", "parity", "--force")
	if err != nil {
		t.Fatalf("read imported state: %v\n%s", err, output)
	}
	for _, expected := range []string{"Import task", "#JAF-123"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("imported status = %q, want %s", output, expected)
		}
	}
	output, err = runFlow(t, harness, "notes", "11111111-1111-4111-8111-111111111111")
	if err != nil || !strings.Contains(output, "DECISION: Keep UUID") {
		t.Fatalf("imported notes = %q, err %v; want one decision", output, err)
	}
}

func TestMigrationKeepsObservedUrgencyOutOfSelection(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "parity",
    "description": "Imported task",
    "status": "pending",
    "priority": "M",
    "urgency": 99.0
  }
]`))

	if output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply"); err != nil {
		t.Fatalf("apply migration: %v\n%s", err, output)
	}
	output, err := runFlow(t, harness, "next", "parity", "--format", "json")
	if err != nil {
		t.Fatalf("read migrated urgency: %v\n%s", err, output)
	}
	var report struct {
		Records []struct {
			Urgency float64 `json:"urgency"`
		} `json:"records"`
	}
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode migrated urgency: %v\n%s", err, output)
	}
	if len(report.Records) != 1 {
		t.Fatalf("migrated records = %d, want one task", len(report.Records))
	}
	if report.Records[0].Urgency >= 10 || report.Records[0].Urgency == 99 {
		t.Fatalf("migrated urgency = %.1f, want derived score", report.Records[0].Urgency)
	}
}

func TestMigrationIsolatedByProject(t *testing.T) {
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

	output, err := runFlow(t, first, "migrate", "taskwarrior", "--source", source, "--apply")
	if err != nil {
		t.Fatalf("apply alpha migration: %v\n%s", err, output)
	}
	output, err = runFlow(t, first, "status", "private", "--force")
	if err != nil || !strings.Contains(output, "Alpha imported task") {
		t.Fatalf("alpha status = %q, err %v; want imported task", output, err)
	}
	output, err = runFlow(t, second, "status", "--force")
	if err != nil {
		t.Fatalf("read beta status: %v\n%s", err, output)
	}
	if strings.Contains(output, "Alpha imported task") {
		t.Fatalf("beta observed alpha migration: %q", output)
	}
}

func TestMigrationTransfersFocusAndSessionNote(t *testing.T) {
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

	output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--legacy-data-dir", legacyDir, "--apply")
	if err != nil {
		t.Fatalf("apply focus migration: %v\n%s", err, output)
	}
	output, err = runFlow(t, harness, "focus")
	if err != nil || !strings.Contains(output, "Initiative: parity") || !strings.Contains(output, "Task: 11111111") {
		t.Fatalf("migrated focus = %q, err %v; want parity anchor", output, err)
	}
	output, err = runFlow(t, harness, "session", "resume")
	if err != nil || !strings.Contains(output, "Resume imported task.") {
		t.Fatalf("migrated session note = %q, err %v; want handoff note", output, err)
	}
}

func TestMigrationMakesLegacySessionsVisibleInNativeList(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "pending-session")
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
	for _, sessionID := range []string{"pending-session", "ack-session"} {
		if err := os.WriteFile(filepath.Join(legacyDir, "focus-"+sessionID+".json"), []byte(`{
  "focused_plan": "parity",
  "focused_task_uuid": "11111111-1111-4111-8111-111111111111",
  "task_track": [],
  "plans_of_interest": []
}`), 0o600); err != nil {
			t.Fatalf("write legacy focus %s: %v", sessionID, err)
		}
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "session-note-pending-session.md"), []byte("Resume pending task.\n"), 0o600); err != nil {
		t.Fatalf("write pending session note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "session-note-ack-session.md"), []byte("Resume acknowledged task.\nacknowledged: 2026-10-05T00:00:00Z\n"), 0o600); err != nil {
		t.Fatalf("write acknowledged session note: %v", err)
	}

	output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--legacy-data-dir", legacyDir, "--apply")
	if err != nil {
		t.Fatalf("apply session migration: %v\n%s", err, output)
	}
	output, err = runFlow(t, harness, "session", "list")
	if err != nil {
		t.Fatalf("list migrated sessions: %v\n%s", err, output)
	}
	if !strings.Contains(output, "pending-session") || !strings.Contains(output, "ack-session") {
		t.Fatalf("migrated session list = %q, want both session IDs", output)
	}
	if !strings.Contains(output, "parity") || !strings.Contains(output, "11111111") {
		t.Fatalf("migrated session list = %q, want native plan and task", output)
	}
	if !strings.Contains(output, "pending") || !strings.Contains(output, "acknowledged") {
		t.Fatalf("migrated session list = %q, want pending and acknowledged handoffs", output)
	}

	output, err = runFlow(t, harness, "focus")
	if err != nil || !strings.Contains(output, "Initiative: parity") || !strings.Contains(output, "Task: 11111111") {
		t.Fatalf("migrated session focus = %q, err %v; want native anchor", output, err)
	}
}

func TestMigrationSessionStateIsolatedByProject(t *testing.T) {
	first := testharness.NewHarness(t, "project-alpha", "alpha-session")
	second := testharness.NewHarness(t, "project-beta", "beta-session")
	source := first.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "alpha-plan",
    "description": "Alpha imported task",
    "status": "pending"
  }
]`))
	legacyDir := filepath.Join(first.Root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("create alpha legacy state directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "focus-alpha-session.json"), []byte(`{
  "focused_plan": "alpha-plan",
  "focused_task_uuid": "11111111-1111-4111-8111-111111111111",
  "task_track": [],
  "plans_of_interest": []
}`), 0o600); err != nil {
		t.Fatalf("write alpha legacy focus: %v", err)
	}

	output, err := runFlow(t, first, "migrate", "taskwarrior", "--source", source, "--legacy-data-dir", legacyDir, "--apply")
	if err != nil {
		t.Fatalf("apply alpha session migration: %v\n%s", err, output)
	}
	output, err = runFlow(t, first, "session", "list")
	if err != nil || !strings.Contains(output, "alpha-session") {
		t.Fatalf("alpha session list = %q, err %v; want migrated session", output, err)
	}

	output, err = runFlow(t, second, "session", "list")
	if err != nil {
		t.Fatalf("list beta sessions: %v\n%s", err, output)
	}
	if strings.Contains(output, "alpha-session") || strings.Contains(output, "alpha-plan") {
		t.Fatalf("beta session list crossed project boundary: %q", output)
	}
}

func TestMigrationRejectsConflictingModes(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte("[]"))

	output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply", "--dry-run")
	if err == nil || !strings.Contains(output, "cannot be combined") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("conflicting migration flags = %q, err %v; want actionable error", output, err)
	}
}

func TestMigrationMapsTaskBucketsAndProjectsInactiveTasksOutOfTree(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "foo",
    "description": "First",
    "status": "pending"
  },
  {
    "uuid": "22222222-2222-4222-8222-222222222222",
    "project": "foo:_archive",
    "description": "Archived",
    "status": "pending",
    "depends": ["11111111-1111-4111-8111-111111111111"]
  },
  {
    "uuid": "33333333-3333-4333-8333-333333333333",
    "project": "foo",
    "description": "Third",
    "status": "pending",
    "depends": ["22222222-2222-4222-8222-222222222222"]
  },
  {
    "uuid": "44444444-4444-4444-8444-444444444444",
    "project": "foo",
    "description": "Fourth",
    "status": "pending",
    "depends": ["33333333-3333-4333-8333-333333333333"]
  },
  {
    "uuid": "55555555-5555-4555-8555-555555555555",
    "project": "foo:_trash",
    "description": "Trash",
    "status": "pending"
  }
]`))

	if output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply"); err != nil {
		t.Fatalf("apply bucket migration: %v\n%s", err, output)
	}

	output, err := runFlow(t, harness, "status", "foo", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("read bucket migration: %v\n%s", err, output)
	}
	var report struct {
		Records []struct {
			ID         string `json:"id"`
			Initiative string `json:"initiative"`
			Status     string `json:"status"`
			Bucket     string `json:"bucket"`
		} `json:"records"`
	}
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode bucket report: %v\n%s", err, output)
	}
	if len(report.Records) != 5 {
		t.Fatalf("bucket records = %d, want 5\n%s", len(report.Records), output)
	}
	wantBuckets := map[string]string{
		"11111111-1111-4111-8111-111111111111": "active",
		"22222222-2222-4222-8222-222222222222": "archive",
		"33333333-3333-4333-8333-333333333333": "active",
		"44444444-4444-4444-8444-444444444444": "active",
		"55555555-5555-4555-8555-555555555555": "trash",
	}
	for _, record := range report.Records {
		if record.Initiative != "foo" {
			t.Fatalf("task %s initiative = %q, want foo", record.ID, record.Initiative)
		}
		if record.Bucket != wantBuckets[record.ID] {
			t.Fatalf("task %s bucket = %q, want %q", record.ID, record.Bucket, wantBuckets[record.ID])
		}
		if record.Status != "pending" {
			t.Fatalf("task %s status = %q, want pending", record.ID, record.Status)
		}
	}

	tree, err := runFlow(t, harness, "tree", "foo")
	if err != nil {
		t.Fatalf("read bucket tree: %v\n%s", err, tree)
	}
	visible := make([]string, 0, 3)
	for _, line := range strings.Split(tree, "\n") {
		for _, description := range []string{"First", "Third", "Fourth"} {
			if strings.Contains(line, description) {
				visible = append(visible, description)
			}
		}
	}
	if strings.Join(visible, " -> ") != "First -> Third -> Fourth" {
		t.Fatalf("bucket tree = %q, want First -> Third -> Fourth", tree)
	}
	for _, hidden := range []string{"Archived", "Trash", "BLOCKED"} {
		if strings.Contains(tree, hidden) {
			t.Fatalf("bucket tree contains inactive marker %q: %q", hidden, tree)
		}
	}
}
