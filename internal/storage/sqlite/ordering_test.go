package sqlite_test

import (
	"context"
	"testing"
)

func TestReplacePendingTaskOrderPersistsInitiativeSlots(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/jaflow.sqlite3")
	initiative := createTestInitiative(t, store)
	first := createTestTask(t, store, initiative.ID, "First")
	second := createTestTask(t, store, initiative.ID, "Second")
	third := createTestTask(t, store, initiative.ID, "Third")
	fourth := createTestTask(t, store, initiative.ID, "Fourth")

	if err := store.RecordOutcome(ctx, first.ID, "First complete"); err != nil {
		t.Fatalf("record first outcome: %v", err)
	}
	if err := store.CompleteTask(ctx, first.ID); err != nil {
		t.Fatalf("complete first task: %v", err)
	}
	if err := store.ReplacePendingTaskOrder(ctx, initiative.ID, []string{fourth.ID, second.ID, third.ID}); err != nil {
		t.Fatalf("replace pending task order: %v", err)
	}

	tasks, err := store.ListTasks(ctx, initiative.ProjectID, initiative.Name)
	if err != nil {
		t.Fatalf("list ordered tasks: %v", err)
	}
	want := []string{first.ID, fourth.ID, second.ID, third.ID}
	if len(tasks) != len(want) {
		t.Fatalf("ordered task count = %d, want %d", len(tasks), len(want))
	}
	for index, taskID := range want {
		if tasks[index].ID != taskID {
			t.Fatalf("ordered task %d = %s, want %s", index, tasks[index].ID, taskID)
		}
	}
	if tasks[0].Position != 1 {
		t.Fatalf("completed task position = %d, want 1", tasks[0].Position)
	}
}
