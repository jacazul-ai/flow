package sqlite_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

func TestLifecycleEnforcesDependenciesAndOutcome(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/flow.sqlite3")
	initiative := createTestInitiative(t, store)
	first := createTestTask(t, store, initiative.ID, "First")
	second := createTestTaskWithDependency(t, store, initiative.ID, "Second", first.ID)

	if err := store.StartTask(ctx, second.ID); err == nil {
		t.Fatal("blocked task started unexpectedly")
	} else if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("blocked task error = %q, want blocked guidance", err)
	}
	if err := store.StartTask(ctx, first.ID); err != nil {
		t.Fatalf("start first task: %v", err)
	}
	if err := store.CompleteTask(ctx, first.ID); err == nil {
		t.Fatal("task completed without outcome")
	} else if !strings.Contains(err.Error(), "OUTCOME") {
		t.Fatalf("missing outcome error = %q, want OUTCOME guidance", err)
	}
	if err := store.RecordOutcome(ctx, first.ID, "First task is complete."); err != nil {
		t.Fatalf("record outcome: %v", err)
	}
	if err := store.CompleteTask(ctx, first.ID); err != nil {
		t.Fatalf("complete first task: %v", err)
	}

	ready, err := store.ReadyTasks(ctx, "project-alpha", "parity")
	if err != nil {
		t.Fatalf("list ready tasks: %v", err)
	}
	if len(ready) != 1 || ready[0].ID != second.ID {
		t.Fatalf("ready tasks = %#v, want second task %q", ready, second.ID)
	}
}

func TestLifecycleSupportsReopenAndDiscard(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/flow.sqlite3")
	initiative := createTestInitiative(t, store)
	reopenable := createTestTask(t, store, initiative.ID, "Reopenable")
	discardable := createTestTask(t, store, initiative.ID, "Discardable")

	if err := store.RecordOutcome(ctx, reopenable.ID, "Completed once."); err != nil {
		t.Fatalf("record reopenable outcome: %v", err)
	}
	if err := store.CompleteTask(ctx, reopenable.ID); err != nil {
		t.Fatalf("complete reopenable task: %v", err)
	}
	if err := store.ReopenTask(ctx, reopenable.ID); err != nil {
		t.Fatalf("reopen task: %v", err)
	}

	if err := store.DiscardTask(ctx, discardable.ID); err != nil {
		t.Fatalf("discard task: %v", err)
	}
	current, err := store.GetTask(ctx, discardable.ID)
	if err != nil {
		t.Fatalf("read discarded task: %v", err)
	}
	if current.Disposition != "discarded" {
		t.Fatalf("disposition = %q, want discarded", current.Disposition)
	}
	if current.Outcome == "" {
		t.Fatal("discarded task has no audit outcome")
	}
}

func createTestInitiative(t *testing.T, store *sqlite.Store) task.Initiative {
	t.Helper()
	initiative, err := store.GetOrCreateInitiative(context.Background(), task.CreateInitiativeInput{
		ProjectID: "project-alpha",
		Name:      "parity",
	})
	if err != nil {
		t.Fatalf("create test initiative: %v", err)
	}
	return initiative
}

func createTestTask(t *testing.T, store *sqlite.Store, initiativeID string, description string) task.Task {
	t.Helper()
	return createTestTaskWithDependency(t, store, initiativeID, description)
}

func createTestTaskWithDependency(t *testing.T, store *sqlite.Store, initiativeID string, description string, dependencies ...string) task.Task {
	t.Helper()
	created, err := store.CreateTask(context.Background(), task.CreateTaskInput{
		InitiativeID: initiativeID,
		Description:  description,
		Dependencies: dependencies,
	})
	if err != nil {
		t.Fatalf("create test task: %v", err)
	}
	return created
}
