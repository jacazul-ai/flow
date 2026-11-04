package flow_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

// Every text output shows the task like the reference tw-flow, which kept the
// mode as a description prefix: "[DESIGN] Design schema".
func TestEveryTaskOutputShowsTheModeBesideTheDescription(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")
	foreign := *harness
	foreign.ProjectID = "foreign"

	output := mustFlow(t, harness, "plan", "modes", "DESIGN|Design schema", "EXECUTE|Implement store")
	ids := createdTaskIDs(output)
	if len(ids) != 2 {
		t.Fatalf("plan output = %q, want two tasks", output)
	}
	design, execute := ids[0], ids[1]
	for _, want := range []string{"Created task " + design + ": [DESIGN] Design schema", "Created task " + execute + ": [EXECUTE] Implement store"} {
		if !strings.Contains(output, want) {
			t.Fatalf("plan output = %q, want %q", output, want)
		}
	}
	mustFlow(t, harness, "plan", "plain", "Plain task", "[PLAN] Map the cutover")
	mustFlow(t, &foreign, "plan", "foreign-modes", "REVIEW|Foreign review")

	checks := []struct {
		name string
		args []string
		want []string
	}{
		{"status", []string{"status", "modes", "--force"}, []string{"[DESIGN] Design schema", "[EXECUTE] Implement store"}},
		{"next", []string{"next", "modes"}, []string{design + " [DESIGN] Design schema"}},
		{"tree", []string{"tree", "modes"}, []string{"[DESIGN] Design schema", "[EXECUTE] Implement store"}},
		{"focus plan", []string{"focus", "plan", "modes"}, []string{"Next task " + design + ": [DESIGN] Design schema"}},
		{"context", []string{"context", design}, []string{"CONTEXT FOR TASK " + design + ": [DESIGN] Design schema"}},
		{"blocked", []string{"blocked", "modes"}, []string{"[EXECUTE] Implement store"}},
	}
	for _, check := range checks {
		output := mustFlow(t, harness, check.args...)
		for _, want := range check.want {
			if !strings.Contains(output, want) {
				t.Fatalf("%s = %q, want %q", check.name, output, want)
			}
		}
	}

	mustFlow(t, harness, "execute", design)
	mustFlow(t, harness, "outcome", design, "Schema designed")
	if output := mustFlow(t, harness, "done", design); !strings.Contains(output, "Ready task "+execute+": [EXECUTE] Implement store") {
		t.Fatalf("done = %q, want the ready task with its mode", output)
	}
	if output := mustFlow(t, harness, "context", execute); !strings.Contains(output, "Task ("+design+") [[DESIGN] Design schema]:") {
		t.Fatalf("context = %q, want the inherited task with its mode", output)
	}
	if output := mustFlow(t, harness, "focus", "task", execute); !strings.Contains(output, "Focused task "+execute+": [EXECUTE] Implement store") {
		t.Fatalf("focus task = %q, want the task with its mode", output)
	}
	if output := mustFlow(t, harness, "onboard"); !strings.Contains(output, "Task: "+execute+" [EXECUTE] Implement store") {
		t.Fatalf("onboard = %q, want the focused task with its mode", output)
	}
	if output := mustFlow(t, harness, "session", "dump"); !strings.Contains(output, "**Task:** `"+execute+"` [EXECUTE] Implement store") {
		t.Fatalf("session dump = %q, want the task with its mode", output)
	}
	mustFlow(t, harness, "execute", execute)
	if output := mustFlow(t, harness, "active", "modes"); !strings.Contains(output, "[EXECUTE] Implement store") {
		t.Fatalf("active = %q, want the task with its mode", output)
	}

	plain := mustFlow(t, harness, "status", "plain", "--force")
	if !regexp.MustCompile(`\] Plain task\n`).MatchString(plain) || strings.Count(plain, "[PLAN]") != 1 || strings.Contains(plain, "UNSPECIFIED") {
		t.Fatalf("status plain = %q, want unmoded tasks as written", plain)
	}

	readout := section(t, mustFlow(t, harness, "ponder", "--force"), "TACTICAL READOUT")
	for _, line := range readout {
		if strings.Contains(line, "[EXECUTE]") || strings.Contains(line, "[DESIGN]") {
			t.Fatalf("ponder readout = %q, want the reference MODE column without a prefix", line)
		}
	}
	if structured := mustFlow(t, harness, "status", "modes", "--format", "json", "--force"); !strings.Contains(structured, `"description":"Implement store"`) || !strings.Contains(structured, `"mode":"EXECUTE"`) {
		t.Fatalf("structured status = %q, want separate mode and description", structured)
	}

	if output := mustFlow(t, &foreign, "status", "--force"); strings.Contains(output, "Design schema") || !strings.Contains(output, "[REVIEW] Foreign review") {
		t.Fatalf("foreign status = %q, want only its own moded task", output)
	}
	if output := mustFlow(t, harness, "status", "--force"); strings.Contains(output, "Foreign review") {
		t.Fatalf("status = %q, leaked the foreign project", output)
	}
}
