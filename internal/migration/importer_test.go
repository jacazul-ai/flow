package migration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/migration"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

const (
	firstUUID  = "11111111-1111-4111-8111-111111111111"
	secondUUID = "22222222-2222-4222-8222-222222222222"
)

func TestBuildBundleMapsLegacyWorkflowState(t *testing.T) {
	depends, err := json.Marshal([]string{"1"})
	if err != nil {
		t.Fatalf("encode dependency fixture: %v", err)
	}
	bundle, warnings, err := migration.BuildBundle("project-alpha", []migration.LegacyTask{
		{
			ID:             1,
			UUID:           firstUUID,
			Project:        "parity",
			Description:    "[EXECUTE] Import the task",
			Status:         "pending",
			Entry:          "20260830T120000Z",
			Modified:       "20260830T120100Z",
			ExternalTicket: "#JAF-123",
			Priority:       "H",
			Urgency:        17.5,
			Annotations: []migration.LegacyAnnotation{{
				Entry:       "20260830T120200Z",
				Description: "DECISION: Keep native identity",
			}},
		},
		{
			ID:          2,
			UUID:        secondUUID,
			Project:     "parity",
			Description: "[TEST] Verify the task",
			Status:      "pending",
			Depends:     depends,
			Wait:        "20990101",
			Entry:       "20260830T120300Z",
			Modified:    "20260830T120400Z",
		},
	})
	if err != nil {
		t.Fatalf("build import bundle: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("bundle warnings = %v, want none", warnings)
	}
	if len(bundle.Initiatives) != 1 || bundle.Initiatives[0].Name != "parity" {
		t.Fatalf("initiatives = %#v, want parity initiative", bundle.Initiatives)
	}
	if len(bundle.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(bundle.Tasks))
	}
	first, second := bundle.Tasks[0], bundle.Tasks[1]
	if first.ID != firstUUID || first.Mode != task.ModeExecute || first.ExternalTicket != "#JAF-123" {
		t.Fatalf("first task = %#v, want mapped identity/mode/ticket", first)
	}
	if second.ID != secondUUID || second.Mode != task.ModeTest || second.WaitUntil != "2099-01-01" {
		t.Fatalf("second task = %#v, want mapped identity/mode/wait", second)
	}
	if len(bundle.Dependencies) != 1 || bundle.Dependencies[0].DependsOnID != firstUUID {
		t.Fatalf("dependencies = %#v, want first UUID target", bundle.Dependencies)
	}
	if len(bundle.Annotations) != 1 || bundle.Annotations[0].Kind != "DECISION" {
		t.Fatalf("annotations = %#v, want DECISION", bundle.Annotations)
	}
}

func TestDryRunDoesNotRequireOrMutateAStore(t *testing.T) {
	bundle, _, err := migration.BuildBundle("project-alpha", []migration.LegacyTask{{
		UUID:        firstUUID,
		Project:     "parity",
		Description: "Task",
		Status:      "pending",
	}})
	if err != nil {
		t.Fatalf("build dry-run bundle: %v", err)
	}
	result, err := migration.NewImporter(nil).DryRun(context.Background(), bundle)
	if err != nil {
		t.Fatalf("dry-run import: %v", err)
	}
	if result.Created != 1 || result.Annotations != 0 {
		t.Fatalf("dry-run result = %#v, want one planned task", result)
	}
}

func TestApplyIsIdempotentAndPreservesDependenciesAndAnnotations(t *testing.T) {
	depends, err := json.Marshal([]string{firstUUID})
	if err != nil {
		t.Fatalf("encode dependency fixture: %v", err)
	}
	bundle, _, err := migration.BuildBundle("project-alpha", []migration.LegacyTask{
		{
			UUID:        firstUUID,
			Project:     "parity",
			Description: "First",
			Status:      "pending",
			Annotations: []migration.LegacyAnnotation{{
				Entry:       "20260830T120000Z",
				Description: "OUTCOME: First complete",
			}},
			History: []migration.LegacyHistoryEvent{{
				SourceEventID: "42",
				Sequence:      42,
				EventType:     "update",
				Property:      "status",
				OldValue:      "pending",
				NewValue:      "completed",
				OccurredAt:    "20260830T120500Z",
			}},
		},
		{
			UUID:        secondUUID,
			Project:     "parity",
			Description: "Second",
			Status:      "pending",
			Depends:     depends,
		},
	})
	if err != nil {
		t.Fatalf("build idempotent bundle: %v", err)
	}
	store, err := sqlite.Open(context.Background(), t.TempDir()+"/jaflow.sqlite3")
	if err != nil {
		t.Fatalf("open import store: %v", err)
	}
	defer store.Close()
	importer := migration.NewImporter(store)
	if _, err := importer.Apply(context.Background(), bundle); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if _, err := importer.Apply(context.Background(), bundle); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	tasks, err := store.ListTasks(context.Background(), "project-alpha", "parity")
	if err != nil {
		t.Fatalf("list imported tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("imported tasks = %d, want 2", len(tasks))
	}
	ready, err := store.ReadyTasks(context.Background(), "project-alpha", "parity")
	if err != nil {
		t.Fatalf("list imported ready tasks: %v", err)
	}
	if len(ready) != 1 || ready[0].ID != firstUUID {
		t.Fatalf("ready tasks = %#v, want only first task", ready)
	}
	annotations, err := store.ListAnnotations(context.Background(), firstUUID)
	if err != nil {
		t.Fatalf("list imported annotations: %v", err)
	}
	if len(annotations) != 1 || annotations[0].Body != "First complete" {
		t.Fatalf("annotations = %#v, want one outcome", annotations)
	}
	if len(tasks[1].Dependencies) != 1 || tasks[1].Dependencies[0] != firstUUID {
		t.Fatalf("dependencies = %#v, want first UUID", tasks[1].Dependencies)
	}
	history, err := store.ListHistory(context.Background(), firstUUID)
	if err != nil {
		t.Fatalf("list imported history: %v", err)
	}
	if len(history) != 1 || history[0].SourceEventID != "42" {
		t.Fatalf("imported history = %#v, want one source event", history)
	}
}

func TestBuildBundleRejectsUnresolvedDependency(t *testing.T) {
	depends, err := json.Marshal([]string{"999"})
	if err != nil {
		t.Fatalf("encode dependency fixture: %v", err)
	}
	_, _, err = migration.BuildBundle("project-alpha", []migration.LegacyTask{{
		ID:          1,
		UUID:        firstUUID,
		Project:     "parity",
		Description: "Task",
		Status:      "pending",
		Depends:     depends,
	}})
	if err == nil || !strings.Contains(err.Error(), "unresolved") {
		t.Fatalf("unresolved dependency error = %v, want actionable failure", err)
	}
}
