package flow_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

func TestMigrationPreservesDisplaysFiltersAndUpdatesStoredTags(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	const taskID = "11111111-1111-4111-8111-111111111111"
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "parity",
    "description": "Tagged task",
    "status": "pending",
    "tags": ["moloide", "research", "moloide"]
  },
  {
    "uuid": "22222222-2222-4222-8222-222222222222",
    "project": "parity",
    "description": "Other task",
    "status": "pending",
    "tags": ["legacy"]
  }
]`))

	for range 2 {
		if output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply"); err != nil {
			t.Fatalf("apply migration: %v\n%s", err, output)
		}
	}

	output, err := runFlow(t, harness, "status", "parity", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("read imported tags: %v\n%s", err, output)
	}
	var report struct {
		Records []struct {
			ID   string   `json:"id"`
			Tags []string `json:"tags"`
		} `json:"records"`
	}
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode tag report: %v\n%s", err, output)
	}
	if len(report.Records) != 2 {
		t.Fatalf("tag records = %d, want 2", len(report.Records))
	}
	if got, want := report.Records[0].Tags, []string{"moloide", "research"}; !sameStrings(got, want) {
		t.Fatalf("imported tags = %#v, want %#v", got, want)
	}

	output, err = runFlow(t, harness, "status", "parity", "+moloide", "-legacy", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("filter by tags: %v\n%s", err, output)
	}
	if !strings.Contains(output, taskID) || strings.Contains(output, "22222222-2222-4222-8222-222222222222") {
		t.Fatalf("tag filter output = %q, want only the moloide task", output)
	}
	output, err = runFlow(t, harness, "status", "parity", "+moloide", "--force")
	if err != nil || !strings.Contains(output, "+moloide") || !strings.Contains(output, "Tagged task") {
		t.Fatalf("text tag display = %q, err %v", output, err)
	}

	if output, err := runFlow(t, harness, "amend", taskID, "+urgent", "-research"); err != nil {
		t.Fatalf("amend tags: %v\n%s", err, output)
	}
	output, err = runFlow(t, harness, "status", "parity", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("read amended tags: %v\n%s", err, output)
	}
	if !strings.Contains(output, `"tags":["moloide","urgent"]`) {
		t.Fatalf("amended tag output = %q, want moloide and urgent", output)
	}
	if strings.Contains(output, `"tags":["moloide","research"]`) {
		t.Fatalf("amended tag output retained removed tag: %q", output)
	}
}

func TestTagFiltersUseDistinctCacheKeysAndRejectConflictingAmendments(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "44444444-4444-4444-8444-444444444444",
    "project": "parity",
    "description": "Research task",
    "status": "pending",
    "tags": ["research"]
  },
  {
    "uuid": "55555555-5555-4555-8555-555555555555",
    "project": "parity",
    "description": "Legacy task",
    "status": "pending",
    "tags": ["legacy"]
  }
]`))
	if output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply"); err != nil {
		t.Fatalf("apply tag filter fixture: %v\n%s", err, output)
	}

	output, err := runFlow(t, harness, "status", "+research")
	if err != nil || !strings.Contains(output, "Research task") || strings.Contains(output, "Legacy task") {
		t.Fatalf("research filter = %q, err %v", output, err)
	}
	output, err = runFlow(t, harness, "status", "+legacy")
	if err != nil || !strings.Contains(output, "Legacy task") || strings.Contains(output, "Research task") {
		t.Fatalf("legacy filter = %q, err %v", output, err)
	}

	if output, err := runFlow(t, harness, "amend", "44444444", "+research", "-research"); err == nil || !strings.Contains(output, "cannot be added and removed") {
		t.Fatalf("conflicting tag amendment = %q, err %v; want actionable rejection", output, err)
	}
}

func TestMigrationConsumesLegacyDiscardedTagIntoDisposition(t *testing.T) {
	harness := testharness.NewHarness(t, "project-alpha", "session")
	source := harness.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "33333333-3333-4333-8333-333333333333",
    "project": "parity",
    "description": "Discarded task",
    "status": "completed",
    "tags": ["DISCARDED", "legacy"]
  }
]`))

	if output, err := runFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply"); err != nil {
		t.Fatalf("apply discarded migration: %v\n%s", err, output)
	}
	output, err := runFlow(t, harness, "status", "parity", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("read discarded task: %v\n%s", err, output)
	}
	if !strings.Contains(output, `"disposition":"discarded"`) {
		t.Fatalf("discarded output = %q, want native disposition", output)
	}
	if !strings.Contains(output, `"tags":["legacy"]`) || strings.Contains(output, "DISCARDED") {
		t.Fatalf("discarded output = %q, want only stored legacy tag", output)
	}
}

func TestStoredTagsAreIsolatedByProject(t *testing.T) {
	first := testharness.NewHarness(t, "project-alpha", "session")
	second := testharness.NewHarness(t, "project-beta", "session")
	source := first.WriteFile(t, "source.json", []byte(`[
  {
    "uuid": "11111111-1111-4111-8111-111111111111",
    "project": "parity",
    "description": "Alpha tagged task",
    "status": "pending",
    "tags": ["private"]
  }
]`))

	if output, err := runFlow(t, first, "migrate", "taskwarrior", "--source", source, "--apply"); err != nil {
		t.Fatalf("apply alpha migration: %v\n%s", err, output)
	}
	output, err := runFlow(t, second, "status", "+private", "--format", "json", "--force")
	if err != nil {
		t.Fatalf("filter beta tags: %v\n%s", err, output)
	}
	if strings.Contains(output, "Alpha tagged task") || strings.Contains(output, "private") {
		t.Fatalf("beta observed alpha tag data: %q", output)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
