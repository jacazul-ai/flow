package flow_test

import (
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

// A tw-flow export stores the mode as a description prefix. The importer
// moves a known prefix into the mode column and keeps an unknown one in the
// description; either way every output shows the prefix exactly once, as the
// reference did.
func TestMigratedTasksShowTheirModePrefixExactlyOnce(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")
	foreign := *harness
	foreign.ProjectID = "foreign"
	source := harness.WriteFile(t, "source.json", []byte(`[
  {"uuid": "30000000-0000-4000-8000-000000000001", "project": "migrated",
   "description": "[DESIGN] Define the schema", "status": "pending", "entry": "20261101T120000Z"},
  {"uuid": "30000000-0000-4000-8000-000000000002", "project": "migrated",
   "description": "[PLAN] Map the cutover", "status": "pending", "entry": "20261101T120000Z",
   "depends": ["30000000-0000-4000-8000-000000000001"]}
]`))
	mustFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply")
	mustFlow(t, &foreign, "plan", "foreign-plan", "DESIGN|Foreign design")

	design, plan := "[DESIGN] Define the schema", "[PLAN] Map the cutover"
	checks := []struct {
		name string
		args []string
		want []string
	}{
		{"status", []string{"status", "migrated", "--force"}, []string{design, plan}},
		{"tree", []string{"tree", "migrated"}, []string{design, plan}},
		{"blocked", []string{"blocked", "migrated"}, []string{plan}},
		{"focus", []string{"focus", "task", "30000000-0000-4000-8000-000000000001"}, []string{design}},
		{"context", []string{"context", "30000000-0000-4000-8000-000000000001"}, []string{design}},
	}
	for _, check := range checks {
		output := mustFlow(t, harness, check.args...)
		for _, want := range check.want {
			if !strings.Contains(output, want) {
				t.Fatalf("%s = %q, want %q", check.name, output, want)
			}
		}
		for _, doubled := range []string{"[DESIGN] [DESIGN]", "[PLAN] [PLAN]", "UNSPECIFIED", "Foreign design"} {
			if strings.Contains(output, doubled) {
				t.Fatalf("%s = %q, want no %q", check.name, output, doubled)
			}
		}
	}
	if output := mustFlow(t, &foreign, "status", "--force"); strings.Contains(output, "Define the schema") || !strings.Contains(output, "[DESIGN] Foreign design") {
		t.Fatalf("foreign status = %q, want only its own task", output)
	}
}
