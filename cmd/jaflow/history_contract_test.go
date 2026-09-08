package main

import (
	"context"
	"strings"
	"testing"

	"github.com/jacazul-ai/jaflow/internal/storage/sqlite"
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

	output, err = runJaflow(t, binary, harness, "history", "task", ids[0])
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

func TestHistoryRejectsImplicitTaskScope(t *testing.T) {
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
	if err == nil || !strings.Contains(output, "history task") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("implicit task history = %q, err %v; want explicit-scope guidance", output, err)
	}
}

func TestHistoryResolvesInitiativeAliasesAndReferences(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	output, err := runJaflow(t, binary, harness, "plan", "history-plan", "History task")
	if err != nil {
		t.Fatalf("create history plan: %v\n%s", err, output)
	}
	initiativeID := initiativeIDForTest(t, harness, "history-plan")
	shortID := initiativeID[:8]
	for _, scope := range []string{"initiative", "ini", "plan"} {
		for _, reference := range []string{"history-plan", initiativeID, shortID} {
			output, err = runJaflow(t, binary, harness, "history", scope, reference)
			if err != nil {
				t.Fatalf("read initiative history with %s %s: %v\n%s", scope, reference, err, output)
			}
			if !strings.Contains(output, "HISTORY: initiative history-plan") ||
				!strings.Contains(output, "id:"+shortID) ||
				!strings.Contains(output, "History task") {
				t.Fatalf("initiative history with %s %s = %q, want resolved UUID", scope, reference, output)
			}
		}
	}
}

func TestInitiativeListingsShowShortUUIDs(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	output, err := runJaflow(t, binary, harness, "plan", "listed-plan", "Listed task")
	if err != nil {
		t.Fatalf("create listed plan: %v\n%s", err, output)
	}
	shortID := initiativeIDForTest(t, harness, "listed-plan")[:8]
	for _, command := range []string{"plans", "inis", "initiatives"} {
		output, err = runJaflow(t, binary, harness, command, "--force")
		if err != nil {
			t.Fatalf("list initiatives through %s: %v\n%s", command, err, output)
		}
		if !strings.Contains(output, "listed-plan") || !strings.Contains(output, "id:"+shortID) {
			t.Fatalf("%s output = %q, want initiative short UUID", command, output)
		}
	}
}

func initiativeIDForTest(t *testing.T, harness *testharness.Harness, name string) string {
	t.Helper()
	store, err := sqlite.Open(context.Background(), harness.DatabasePath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer store.Close()
	summaries, err := store.ListInitiatives(context.Background(), harness.ProjectID, true, true)
	if err != nil {
		t.Fatalf("list test initiatives: %v", err)
	}
	for _, summary := range summaries {
		if summary.Initiative.Name == name {
			return summary.Initiative.ID
		}
	}
	t.Fatalf("initiative %q not found in test database", name)
	return ""
}
