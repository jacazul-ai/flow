package main

import (
	"strings"
	"testing"

	"github.com/jacazul-ai/jaflow/internal/testharness"
)

func TestHistoryReportsTaskAndInitiativeEvents(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	output, err := runJaflow(t, binary, harness, "plan", "history-plan", "History task")
	if err != nil {
		t.Fatalf("create history plan: %v\n%s", err, output)
	}
	ids := createdTaskIDs(output)
	if len(ids) != 1 {
		t.Fatalf("plan output = %q, want one task ID", output)
	}

	output, err = runJaflow(t, binary, harness, "history", ids[0])
	if err != nil {
		t.Fatalf("read task history: %v\n%s", err, output)
	}
	for _, expected := range []string{"HISTORY: task", "create", "History task"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("task history = %q, want %s", output, expected)
		}
	}

	output, err = runJaflow(t, binary, harness, "history", "initiative", "history-plan")
	if err != nil {
		t.Fatalf("read initiative history: %v\n%s", err, output)
	}
	if !strings.Contains(output, "HISTORY: initiative history-plan") || !strings.Contains(output, "History task") {
		t.Fatalf("initiative history = %q, want initiative and task events", output)
	}
}
