package flow_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestMigrationExportsThroughTaskSubprocess(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	export := harness.WriteFile(t, "task-export.json", []byte(`[
  {
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "parity",
    "description": "Subprocess import",
    "status": "pending"
  }
]`))
	argsFile := harness.WriteFile(t, "task-args.txt", nil)
	taskData := filepath.Join(harness.Root, "legacy-taskdata")
	if err := os.MkdirAll(taskData, 0o700); err != nil {
		t.Fatalf("create taskdata directory: %v", err)
	}
	harness.FakeCommand(t, "task", `#!/bin/sh
set -eu
printf '%s\\n' "$@" > "$TASK_ARGS_FILE"
cat "$TASK_EXPORT_FILE"
`)
	t.Setenv("TASK_ARGS_FILE", argsFile)
	t.Setenv("TASK_EXPORT_FILE", export)

	code, stdout, stderr := run(t, context.Background(), flowEnv(harness), "--taskdata", taskData, "migrate", "taskwarrior")
	if code != 0 {
		t.Fatalf("migration exit = %d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read task arguments: %v", err)
	}
	if !strings.Contains(string(args), "rc.data.location="+taskData) || !strings.Contains(string(args), "export") {
		t.Fatalf("task arguments = %q, want explicit data location and export", args)
	}
	if !strings.Contains(stdout, "Records: created=1") {
		t.Fatalf("stdout = %q, want one imported record", stdout)
	}
}

func TestMigrationFailsWhenTaskExecutableIsMissing(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	t.Setenv("PATH", harness.BinDir)

	code, stdout, stderr := run(t, context.Background(), flowEnv(harness), "migrate", "taskwarrior")
	if code != 1 {
		t.Fatalf("migration exit = %d, stdout=%q, stderr=%q; want failure", code, stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "Taskwarrior executable") || !strings.Contains(combined, "ACTION:") {
		t.Fatalf("missing-task error = %q, want executable diagnosis and ACTION", combined)
	}
	if _, err := os.Stat(harness.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("missing-task migration touched database: %v", err)
	}
}

func TestMigrationReportsTaskSubprocessFailure(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	harness.FakeCommand(t, "task", `#!/bin/sh
printf '%s\\n' "Taskwarrior data is unreadable" >&2
exit 7
`)

	code, stdout, stderr := run(t, context.Background(), flowEnv(harness), "migrate", "taskwarrior")
	if code != 1 {
		t.Fatalf("migration exit = %d, stdout=%q, stderr=%q; want failure", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "Taskwarrior export failed") || !strings.Contains(stderr, "Taskwarrior data is unreadable") || !strings.Contains(stderr, "ACTION:") {
		t.Fatalf("subprocess failure = %q, want command diagnostic and ACTION", stderr)
	}
	if _, err := os.Stat(harness.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("failed task export touched database: %v", err)
	}
}

func TestMigrationRejectsInvalidTaskExport(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	harness.FakeCommand(t, "task", `#!/bin/sh
printf '%s\\n' "not-json"
`)

	code, stdout, stderr := run(t, context.Background(), flowEnv(harness), "migrate", "taskwarrior")
	if code != 1 {
		t.Fatalf("migration exit = %d, stdout=%q, stderr=%q; want failure", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "decode Taskwarrior export") || !strings.Contains(stderr, "ACTION:") {
		t.Fatalf("invalid export = %q, want decode diagnosis and ACTION", stderr)
	}
	if _, err := os.Stat(harness.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("invalid task export touched database: %v", err)
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

func TestMigrationBucketApplyIsIdempotent(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "66666666-6666-4666-8666-666666666666",
    "project": "foo:_archive",
    "description": "Archived task",
    "status": "pending"
  },
  {
    "uuid": "77777777-7777-4777-8777-777777777777",
    "project": "foo:_trash",
    "description": "Trash task",
    "status": "pending"
  }
]`))

	for range 2 {
		if output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply"); err != nil {
			t.Fatalf("apply bucket migration: %v\n%s", err, output)
		}
	}
	output, err := runFlow(t, harness, "status", "foo", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("read repeated bucket migration: %v\n%s", err, output)
	}
	var report struct {
		Records []struct {
			ID string `json:"id"`
		} `json:"records"`
	}
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode repeated bucket report: %v\n%s", err, output)
	}
	if len(report.Records) != 2 {
		t.Fatalf("repeated bucket records = %d, want 2\n%s", len(report.Records), output)
	}
}

func TestMigrationApplyPersistsIssuesForInspection(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "issues-source.json", []byte(`[
  {
    "uuid": "12121212-1212-4121-8121-121212121212",
    "project": "foo",
    "description": "Issue task",
    "status": "legacy-status",
    "due": "not-a-date"
  }
]`))

	code, stdout, stderr := run(t, context.Background(), flowEnv(harness), "migrate", "taskwarrior", "--source", source, "--apply")
	if code != 1 || !strings.Contains(stdout+stderr, "ERROR") {
		t.Fatalf("issue migration = code %d, stdout=%q, stderr=%q; want exit 1", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, context.Background(), flowEnv(harness), "migrate", "taskwarrior", "--issues")
	if code != 0 {
		t.Fatalf("issue report = code %d, stdout=%q, stderr=%q; want report", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "WARNING") || !strings.Contains(stdout, "ERROR") {
		t.Fatalf("issue report = %q, want WARNING and ERROR entries", stdout)
	}
}

func TestMigrationWarningsDoNotFailTheRun(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "88888888-8888-4888-8888-888888888888",
    "project": "foo",
    "description": "Unknown status task",
    "status": "legacy-status"
  }
]`))

	code, stdout, stderr := run(t, context.Background(), flowEnv(harness), "migrate", "taskwarrior", "--source", source)
	if code != 0 {
		t.Fatalf("warning-only migration exit = %d, stdout=%q, stderr=%q; want 0", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "WARNING:") || stderr != "" {
		t.Fatalf("warning-only migration output = stdout %q, stderr %q; want warning on stdout", stdout, stderr)
	}
}

func TestMigrationRecordErrorsContinueAndReturnFailure(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "99999999-9999-4999-8999-999999999999",
    "project": "foo",
    "description": "Valid task",
    "status": "pending"
  },
  {
    "uuid": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
    "project": "foo",
    "description": "Invalid date task",
    "status": "pending",
    "due": "not-a-date"
  }
]`))

	code, stdout, stderr := run(t, context.Background(), flowEnv(harness), "migrate", "taskwarrior", "--source", source, "--apply")
	combined := stdout + stderr
	if code != 1 || !strings.Contains(combined, "ERROR") {
		t.Fatalf("record-error migration = code %d, stdout=%q, stderr=%q; want exit 1 and ERROR", code, stdout, stderr)
	}
	output, err := runFlow(t, harness, "status", "foo", "--force")
	if err != nil || !strings.Contains(output, "Valid task") || !strings.Contains(output, "Invalid date task") {
		t.Fatalf("record-error migration state = %q, err %v; want both tasks preserved", output, err)
	}
}

func TestMigrationPreservesLifecycleAndBucketMapping(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "lifecycle-source.json", []byte(`[
  {
    "uuid": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
    "project": "foo:_archive",
    "description": "Archived pending task",
    "status": "pending"
  },
  {
    "uuid": "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
    "project": "foo:_trash",
    "description": "Trash pending task",
    "status": "pending"
  },
  {
    "uuid": "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
    "project": "foo",
    "description": "Waiting task",
    "status": "waiting",
    "wait": "20990101"
  },
  {
    "uuid": "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
    "project": "foo",
    "description": "Recurring task",
    "status": "recurring"
  },
  {
    "uuid": "ffffffff-ffff-4fff-8fff-ffffffffffff",
    "project": "foo",
    "description": "Deleted task",
    "status": "deleted"
  }
]`))

	if output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply"); err != nil {
		t.Fatalf("apply lifecycle migration: %v\n%s", err, output)
	}
	output, err := runFlow(t, harness, "status", "foo", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("read lifecycle migration: %v\n%s", err, output)
	}
	var report struct {
		Records []struct {
			ID          string `json:"id"`
			Bucket      string `json:"bucket"`
			Status      string `json:"status"`
			Disposition string `json:"disposition"`
		} `json:"records"`
	}
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode lifecycle report: %v\n%s", err, output)
	}
	want := map[string]struct {
		bucket      string
		status      string
		disposition string
	}{
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb": {bucket: "archive", status: "pending"},
		"cccccccc-cccc-4ccc-8ccc-cccccccccccc": {bucket: "trash", status: "pending"},
		"dddddddd-dddd-4ddd-8ddd-dddddddddddd": {bucket: "active", status: "pending"},
		"eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee": {bucket: "active", status: "pending"},
		"ffffffff-ffff-4fff-8fff-ffffffffffff": {bucket: "active", status: "completed", disposition: "discarded"},
	}
	if len(report.Records) != len(want) {
		t.Fatalf("lifecycle records = %d, want %d\n%s", len(report.Records), len(want), output)
	}
	for _, record := range report.Records {
		expected, ok := want[record.ID]
		if !ok {
			t.Fatalf("unexpected lifecycle task %s", record.ID)
		}
		if record.Bucket != expected.bucket || record.Status != expected.status || record.Disposition != expected.disposition {
			t.Fatalf("lifecycle task %s = bucket %q status %q disposition %q, want %#v", record.ID, record.Bucket, record.Status, record.Disposition, expected)
		}
	}
}

func TestMigrationCanonicalLauncherSnapshotPreservesShape(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "canonical-source.json", canonicalLauncherSnapshot(t))

	output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--dry-run")
	if err != nil {
		t.Fatalf("canonical dry-run: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Records: created=1961") {
		t.Fatalf("canonical dry-run = %q, want 1961 planned tasks", output)
	}
	if _, err := os.Stat(harness.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("canonical dry-run created target database: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		output, err = runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply")
		if err != nil {
			t.Fatalf("canonical apply attempt %d: %v\n%s", attempt+1, err, output)
		}
	}

	output, err = runFlow(t, harness, "status", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("canonical status: %v\n%s", err, output)
	}
	var tasks struct {
		Records []struct {
			Bucket string `json:"bucket"`
		} `json:"records"`
	}
	if err := json.Unmarshal([]byte(output), &tasks); err != nil {
		t.Fatalf("decode canonical tasks: %v\n%s", err, output)
	}
	if len(tasks.Records) != 1961 {
		t.Fatalf("canonical tasks = %d, want 1961", len(tasks.Records))
	}
	bucketCounts := map[string]int{}
	for _, record := range tasks.Records {
		bucketCounts[record.Bucket]++
	}
	if bucketCounts["active"] != 1534 || bucketCounts["archive"] != 427 || bucketCounts["trash"] != 0 {
		t.Fatalf("canonical buckets = %#v, want active=1534 archive=427 trash=0", bucketCounts)
	}

	output, err = runFlow(t, harness, "plans", "--all", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("canonical plans: %v\n%s", err, output)
	}
	var plans struct {
		Records []struct {
			Name string `json:"name"`
		} `json:"records"`
	}
	if err := json.Unmarshal([]byte(output), &plans); err != nil {
		t.Fatalf("decode canonical plans: %v\n%s", err, output)
	}
	if len(plans.Records) != 225 {
		t.Fatalf("canonical initiatives = %d, want 225", len(plans.Records))
	}
	for _, plan := range plans.Records {
		if strings.HasSuffix(plan.Name, ":_archive") || strings.HasSuffix(plan.Name, ":_trash") {
			t.Fatalf("canonical plan retained source bucket suffix: %q", plan.Name)
		}
	}
}

func canonicalLauncherSnapshot(t *testing.T) []byte {
	t.Helper()
	pairBases := []string{
		"enforce-outcome", "flow-cached-signal", "jacazul-home", "jacazul-pi",
		"onboard", "skill-agent-modularization", "skill-system-evolution",
		"smoke", "test-feature", "tw-flow-visibility", "workflow_stabilization",
	}
	archiveOnlyBases := []string{
		"urgency-test-v4", "i-d-clarity", "jacazul-go-launcher", "mode-test",
		"pending", "personas", "smoke-debug2", "smoke-debug3",
		"test-backlog-sandbox", "test_del", "test_rename_new", "unscoped",
		"worktree-support-chase",
	}
	normalLabels := append([]string{}, pairBases...)
	for index := 0; len(normalLabels) < 212; index++ {
		normalLabels = append(normalLabels, fmt.Sprintf("canonical-%03d", index))
	}
	archiveLabels := make([]string, 0, 24)
	for _, base := range pairBases {
		archiveLabels = append(archiveLabels, base+":_archive")
	}
	for _, base := range archiveOnlyBases {
		archiveLabels = append(archiveLabels, base+":_archive")
	}

	type sourceTask struct {
		UUID        string   `json:"uuid"`
		Project     string   `json:"project"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		Entry       string   `json:"entry"`
		Modified    string   `json:"modified"`
		End         string   `json:"end,omitempty"`
		Tags        []string `json:"tags,omitempty"`
	}
	tasks := make([]sourceTask, 0, 1961)
	appendTask := func(project, status, description string, index int) {
		task := sourceTask{
			UUID:        fmt.Sprintf("%08x-0000-4000-8000-%012x", index, index),
			Project:     project,
			Description: description,
			Status:      status,
			Entry:       "20260101T000000Z",
			Modified:    "20260101T000100Z",
		}
		if status == "completed" || status == "deleted" {
			task.End = "20260101T000200Z"
		}
		if index%100 == 0 {
			task.Tags = []string{"canonical"}
		}
		tasks = append(tasks, task)
	}

	index := 1
	for archiveIndex := 0; archiveIndex < 427; archiveIndex++ {
		status := "pending"
		if archiveIndex < 404 {
			status = "completed"
		} else if archiveIndex == 404 {
			status = "deleted"
		}
		appendTask(archiveLabels[archiveIndex%len(archiveLabels)], status, fmt.Sprintf("Canonical archive task %04d", archiveIndex), index)
		index++
	}
	for normalIndex := 0; index <= 1961; normalIndex++ {
		appendTask(normalLabels[normalIndex%len(normalLabels)], "pending", fmt.Sprintf("Canonical active task %04d", normalIndex), index)
		index++
	}
	data, err := json.Marshal(tasks)
	if err != nil {
		t.Fatalf("encode canonical launcher snapshot: %v", err)
	}
	return data
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
	for _, hidden := range []string{"Archived", "Trash"} {
		if strings.Contains(tree, hidden) {
			t.Fatalf("bucket tree contains inactive marker %q: %q", hidden, tree)
		}
	}
	for _, line := range strings.Split(tree, "\n") {
		if strings.Contains(line, "Third") && !strings.HasPrefix(line, "[READY]") {
			t.Fatalf("archived dependency still blocks Third: %q", tree)
		}
	}
}
