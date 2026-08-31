package sqlite_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jacazul-ai/jaflow/internal/task"
)

func TestHistorySupportsTaskAndInitiativeScopes(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/jaflow.sqlite3")
	initiative := createTestInitiative(t, store)
	created := createTestTask(t, store, initiative.ID, "History task")

	if err := store.AppendHistoryEvent(ctx, task.HistoryEvent{
		TaskID:        created.ID,
		Source:        "native",
		SourceEventID: "native-task-1",
		EventType:     "update",
		Property:      "description",
		OldValue:      "History task",
		NewValue:      "Updated history task",
		OccurredAt:    "2026-08-30T12:00:00Z",
	}); err != nil {
		t.Fatalf("append task history: %v", err)
	}
	if err := store.AppendHistoryEvent(ctx, task.HistoryEvent{
		InitiativeID:  initiative.ID,
		Source:        "native",
		SourceEventID: "native-initiative-1",
		EventType:     "rename",
		Property:      "name",
		OldValue:      "parity",
		NewValue:      "renamed",
		OccurredAt:    "2026-08-30T12:01:00Z",
	}); err != nil {
		t.Fatalf("append initiative history: %v", err)
	}
	if err := store.AppendHistoryEvent(ctx, task.HistoryEvent{
		TaskID:        created.ID,
		Source:        "native",
		SourceEventID: "native-task-1",
		EventType:     "ignored-duplicate",
		OccurredAt:    "2026-08-30T12:02:00Z",
	}); err != nil {
		t.Fatalf("append duplicate task history: %v", err)
	}

	taskEvents, err := store.ListHistory(ctx, created.ID)
	if err != nil {
		t.Fatalf("list task history: %v", err)
	}
	if len(taskEvents) != 2 {
		t.Fatalf("task history = %#v, want create and description events", taskEvents)
	}
	foundDescription := false
	for _, event := range taskEvents {
		if event.Property == "description" && event.EventType == "update" {
			foundDescription = true
		}
	}
	if !foundDescription {
		t.Fatalf("task history = %#v, missing description event", taskEvents)
	}
	initiativeEvents, err := store.ListInitiativeHistory(ctx, "project-alpha", "parity")
	if err != nil {
		t.Fatalf("list initiative history: %v", err)
	}
	if len(initiativeEvents) != 4 {
		t.Fatalf("initiative history = %#v, want initiative create, task create/update, and rename events", initiativeEvents)
	}
}

func TestLifecycleRecordsNativeHistory(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/jaflow.sqlite3")
	initiative := createTestInitiative(t, store)
	created := createTestTask(t, store, initiative.ID, "Lifecycle history")

	if err := store.StartTask(ctx, created.ID); err != nil {
		t.Fatalf("start task: %v", err)
	}
	if err := store.RecordOutcome(ctx, created.ID, "Finished"); err != nil {
		t.Fatalf("record outcome: %v", err)
	}
	if err := store.CompleteTask(ctx, created.ID); err != nil {
		t.Fatalf("complete task: %v", err)
	}

	events, err := store.ListHistory(ctx, created.ID)
	if err != nil {
		t.Fatalf("list lifecycle history: %v", err)
	}
	if len(events) < 4 {
		t.Fatalf("lifecycle history = %#v, want create/start/outcome/complete", events)
	}
	for _, eventType := range []string{"create", "start", "outcome", "complete"} {
		found := false
		for _, event := range events {
			if event.EventType == eventType {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("lifecycle history = %#v, missing %s event", events, eventType)
		}
	}
}

func TestHistoryRejectsMissingScope(t *testing.T) {
	store := openStore(t, t.TempDir()+"/jaflow.sqlite3")
	err := store.AppendHistoryEvent(context.Background(), task.HistoryEvent{
		Source:        "taskchampion",
		SourceEventID: "orphan",
		EventType:     "update",
		OccurredAt:    "2026-08-30T12:00:00Z",
	})
	if err == nil || !strings.Contains(err.Error(), "task ID or initiative ID") {
		t.Fatalf("missing history scope error = %v, want actionable validation", err)
	}
}
