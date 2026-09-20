package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jacazul-ai/flow/internal/task"
)

func TestFocusIsolatedBySession(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/jaflow.sqlite3")
	state := task.FocusState{
		ProjectID:     "project-alpha",
		SessionID:     "session-one",
		InitiativeID:  "initiative-1",
		FocusedTaskID: "task-1",
		TaskStack: []task.FocusEntry{{
			TaskID:       "task-1",
			InitiativeID: "initiative-1",
		}},
		PlansOfInterest: []string{"initiative-1"},
	}
	if err := store.SaveFocus(ctx, state); err != nil {
		t.Fatalf("save focus: %v", err)
	}

	loaded, err := store.LoadFocus(ctx, "project-alpha", "session-one")
	if err != nil {
		t.Fatalf("load focus: %v", err)
	}
	if loaded.FocusedTaskID != state.FocusedTaskID || len(loaded.TaskStack) != 1 {
		t.Fatalf("loaded focus = %#v, want %#v", loaded, state)
	}
	if len(loaded.PlansOfInterest) != 1 || loaded.PlansOfInterest[0] != "initiative-1" {
		t.Fatalf("loaded interests = %#v, want initiative-1", loaded.PlansOfInterest)
	}

	other, err := store.LoadFocus(ctx, "project-alpha", "session-two")
	if err != nil {
		t.Fatalf("load other session focus: %v", err)
	}
	if other.FocusedTaskID != "" || len(other.TaskStack) != 0 {
		t.Fatalf("other session observed focus = %#v", other)
	}
}

func TestCacheIsolatedBySessionAndExpiry(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/jaflow.sqlite3")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.SetCache(ctx, "project-alpha", "session-one", "status", "alpha", now.Add(time.Minute)); err != nil {
		t.Fatalf("set cache: %v", err)
	}

	value, found, err := store.GetCache(ctx, "project-alpha", "session-one", "status", now)
	if err != nil || !found || value != "alpha" {
		t.Fatalf("cache = %q, %t, %v; want alpha, true, nil", value, found, err)
	}
	_, found, err = store.GetCache(ctx, "project-alpha", "session-two", "status", now)
	if err != nil {
		t.Fatalf("get other session cache: %v", err)
	}
	if found {
		t.Fatal("other session observed cache")
	}
	_, found, err = store.GetCache(ctx, "project-alpha", "session-one", "status", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("get expired cache: %v", err)
	}
	if found {
		t.Fatal("expired cache was returned")
	}
}
