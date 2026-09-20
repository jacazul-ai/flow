package main

import (
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

func TestOrganizeReordersPendingTasksWithoutChangingReadiness(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	output, err := runJaflow(
		t,
		binary,
		harness,
		"plan",
		"ordering",
		"One",
		"Two",
		"Three",
		"Four",
		"Five",
		"Six",
	)
	if err != nil {
		t.Fatalf("create ordering plan: %v\n%s", err, output)
	}
	ids := createdTaskIDs(output)
	if len(ids) != 6 {
		t.Fatalf("plan output = %q, want six task IDs", output)
	}
	completeTaskForOrganize(t, binary, harness, ids[0], "One complete")
	completeTaskForOrganize(t, binary, harness, ids[1], "Two complete")

	output, err = runJaflow(t, binary, harness,
		"organize", "order", "ordering", ids[5], ids[3], ids[4], ids[2])
	if err != nil {
		t.Fatalf("partially reorder pending tasks: %v\n%s", err, output)
	}
	assertPendingOrder(t, binary, harness, "ordering", "Six", "Four", "Five", "Three")

	output, err = runJaflow(t, binary, harness, "next", "ordering")
	if err != nil {
		t.Fatalf("list ready tasks after reorder: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Three") ||
		strings.Contains(output, "Four") ||
		strings.Contains(output, "Five") ||
		strings.Contains(output, "Six") {
		t.Fatalf("next after reorder = %q, want only the dependency-ready task", output)
	}

	output, err = runJaflow(t, binary, harness, "organize", "first", "ordering", ids[4])
	if err != nil {
		t.Fatalf("move task first: %v\n%s", err, output)
	}
	assertPendingOrder(t, binary, harness, "ordering", "Five", "Six", "Four", "Three")

	output, err = runJaflow(t, binary, harness, "organize", "after", "ordering", ids[2], ids[4])
	if err != nil {
		t.Fatalf("move task after anchor: %v\n%s", err, output)
	}
	assertPendingOrder(t, binary, harness, "ordering", "Five", "Three", "Six", "Four")

	output, err = runJaflow(t, binary, harness,
		"organize", "block", "ordering", ids[5], ids[3], "--after", ids[4])
	if err != nil {
		t.Fatalf("move task block after anchor: %v\n%s", err, output)
	}
	assertPendingOrder(t, binary, harness, "ordering", "Five", "Six", "Four", "Three")

	output, err = runJaflow(t, binary, harness,
		"organize", "block", "ordering", ids[2], ids[3], "--first")
	if err != nil {
		t.Fatalf("move task block first: %v\n%s", err, output)
	}
	assertPendingOrder(t, binary, harness, "ordering", "Three", "Four", "Five", "Six")
}

func TestOrganizeRejectsUnsafeTaskReferences(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	output, err := runJaflow(t, binary, harness, "plan", "ordering", "One", "Two")
	if err != nil {
		t.Fatalf("create ordering plan: %v\n%s", err, output)
	}
	ids := createdTaskIDs(output)
	if len(ids) != 2 {
		t.Fatalf("plan output = %q, want two task IDs", output)
	}

	output, err = runJaflow(t, binary, harness,
		"organize", "order", "ordering", ids[1], ids[1])
	if err == nil || !strings.Contains(strings.ToLower(output), "duplicate") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("duplicate references = %q, err %v; want actionable rejection", output, err)
	}

	completeTaskForOrganize(t, binary, harness, ids[0], "One complete")
	output, err = runJaflow(t, binary, harness, "organize", "first", "ordering", ids[0])
	if err == nil || !strings.Contains(strings.ToLower(output), "completed") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("completed reference = %q, err %v; want actionable rejection", output, err)
	}

	output, err = runJaflow(t, binary, harness, "plan", "other", "Other task")
	if err != nil {
		t.Fatalf("create other initiative: %v\n%s", err, output)
	}
	foreignIDs := createdTaskIDs(output)
	if len(foreignIDs) != 1 {
		t.Fatalf("other initiative output = %q, want one task ID", output)
	}
	output, err = runJaflow(t, binary, harness,
		"organize", "order", "ordering", ids[1], foreignIDs[0])
	if err == nil || !strings.Contains(strings.ToLower(output), "initiative") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("cross-initiative reference = %q, err %v; want actionable rejection", output, err)
	}

	output, err = runJaflow(t, binary, harness, "execute", ids[1])
	if err != nil {
		t.Fatalf("start second task: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "organize", "first", "ordering", ids[1])
	if err == nil || !strings.Contains(strings.ToLower(output), "active") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("active reference = %q, err %v; want actionable rejection", output, err)
	}
}

func TestOrganizeCannotCrossProjectBoundary(t *testing.T) {
	binary := buildJaflow(t)
	first := testharness.NewHarness(t, "project-alpha", "session-alpha")
	second := testharness.NewHarness(t, "project-beta", "session-beta")

	output, err := runJaflow(t, binary, first, "plan", "ordering", "First", "Second")
	if err != nil {
		t.Fatalf("create first project plan: %v\n%s", err, output)
	}
	firstIDs := createdTaskIDs(output)
	if len(firstIDs) != 2 {
		t.Fatalf("first plan output = %q, want two task IDs", output)
	}
	output, err = runJaflow(t, binary, second, "plan", "other", "Foreign")
	if err != nil {
		t.Fatalf("create second project plan: %v\n%s", err, output)
	}
	secondIDs := createdTaskIDs(output)
	if len(secondIDs) != 1 {
		t.Fatalf("second plan output = %q, want one task ID", output)
	}

	output, err = runJaflow(t, binary, first,
		"organize", "order", "ordering", firstIDs[1], secondIDs[0])
	if err == nil || !strings.Contains(strings.ToLower(output), "not found") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("cross-project reference = %q, err %v; want actionable rejection", output, err)
	}
}

func completeTaskForOrganize(t *testing.T, binary string, harness *testharness.Harness, taskID string, outcome string) {
	t.Helper()
	for _, args := range [][]string{
		{"execute", taskID},
		{"outcome", taskID, outcome},
		{"done", taskID},
	} {
		output, err := runJaflow(t, binary, harness, args...)
		if err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
	}
}

func assertPendingOrder(t *testing.T, binary string, harness *testharness.Harness, initiative string, descriptions ...string) {
	t.Helper()
	output, err := runJaflow(t, binary, harness, "status", initiative, "--force")
	if err != nil {
		t.Fatalf("read organized status: %v\n%s", err, output)
	}
	last := -1
	for _, description := range descriptions {
		index := strings.Index(output, description)
		if index == -1 {
			t.Fatalf("status = %q, want %q", output, description)
		}
		if index < last {
			t.Fatalf("status = %q, want pending order %v", output, descriptions)
		}
		last = index
	}
}
