package sqlite_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

func TestTaskMetadataControlsReadiness(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/jaflow.sqlite3")
	initiative := createTestInitiative(t, store)
	created := createTestTask(t, store, initiative.ID, "Metadata task")

	if created.Priority != "M" {
		t.Fatalf("default priority = %q, want M", created.Priority)
	}
	if err := store.SetTaskUrgency(ctx, created.ID, 18.5); err != nil {
		t.Fatalf("set urgency: %v", err)
	}
	if err := store.SetTaskWait(ctx, created.ID, "2099-01-01"); err != nil {
		t.Fatalf("set wait: %v", err)
	}

	current, err := store.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("read metadata task: %v", err)
	}
	if current.Priority != "H" || current.Urgency != 18.5 {
		t.Fatalf("urgent metadata = priority %q, urgency %v; want H, 18.5", current.Priority, current.Urgency)
	}
	if current.WaitUntil != "2099-01-01" {
		t.Fatalf("wait until = %q, want 2099-01-01", current.WaitUntil)
	}

	ready, err := store.ReadyTasks(ctx, "project-alpha", "parity")
	if err != nil {
		t.Fatalf("list ready tasks: %v", err)
	}
	if len(ready) != 0 {
		t.Fatalf("ready tasks = %#v, want waited task excluded", ready)
	}
}

func TestTaskMetadataUpdateAndDependencyProjectBoundary(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/jaflow.sqlite3")
	initiative := createTestInitiative(t, store)
	created := createTestTask(t, store, initiative.ID, "Original")

	description := "Updated"
	ticket := "#JAF-123"
	updated, err := store.UpdateTaskMetadata(ctx, created.ID, task.TaskMetadataUpdate{
		Description:    &description,
		ExternalTicket: &ticket,
	})
	if err != nil {
		t.Fatalf("update task metadata: %v", err)
	}
	if updated.Description != description || updated.ExternalTicket != ticket {
		t.Fatalf("updated task = %#v, want description and ticket", updated)
	}

	otherInitiative, err := store.GetOrCreateInitiative(ctx, task.CreateInitiativeInput{
		ProjectID: "project-beta",
		Name:      "other",
	})
	if err != nil {
		t.Fatalf("create other project initiative: %v", err)
	}
	other := createTestTask(t, store, otherInitiative.ID, "Other project task")
	if err := store.AddDependency(ctx, created.ID, other.ID); err == nil {
		t.Fatal("cross-project dependency succeeded")
	} else if !strings.Contains(err.Error(), "different projects") {
		t.Fatalf("cross-project dependency error = %q, want boundary guidance", err)
	}
}

func TestRenameInitiativeRejectsCollisions(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, t.TempDir()+"/jaflow.sqlite3")
	createTestInitiative(t, store)
	if _, err := store.GetOrCreateInitiative(ctx, task.CreateInitiativeInput{
		ProjectID: "project-alpha",
		Name:      "other",
	}); err != nil {
		t.Fatalf("create second initiative: %v", err)
	}

	if err := store.RenameInitiative(ctx, "project-alpha", "parity", "other"); err == nil {
		t.Fatal("rename to an existing initiative succeeded")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("rename collision error = %q, want already exists", err)
	}
}

func createTestInitiativeForProject(t *testing.T, store *sqlite.Store, projectID string, name string) task.Initiative {
	t.Helper()
	initiative, err := store.GetOrCreateInitiative(context.Background(), task.CreateInitiativeInput{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		t.Fatalf("create initiative %s/%s: %v", projectID, name, err)
	}
	return initiative
}
