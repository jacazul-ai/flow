package flow_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

func TestStatusResolvesTicketsAcrossDependencyCycles(t *testing.T) {
	created := regexp.MustCompile(`Created task ([0-9a-f]{8})`)
	for attempt := 0; attempt < 100; attempt++ {
		harness := newCycleHarness(t, attempt)
		ids := make(map[string]string, 4)
		for _, description := range []string{"A", "B", "D", "X"} {
			output, err := runFlow(t, harness, "plan", "cyc", description)
			if err != nil {
				t.Fatalf("create %s: %v\n%s", description, err, output)
			}
			match := created.FindStringSubmatch(output)
			if len(match) != 2 {
				t.Fatalf("create %s output = %q, want task UUID", description, output)
			}
			ids[description] = match[1]
		}

		// ListTasks sorts dependency UUIDs. Retry the disposable fixture until
		// B is visited before D, which exposes the path-dependent memo bug.
		if ids["B"] >= ids["D"] {
			continue
		}
		for _, edge := range [][2]string{{"A", "B"}, {"B", "A"}, {"A", "D"}, {"X", "B"}} {
			output, err := runFlow(t, harness, "block", ids[edge[0]], ids[edge[1]])
			if err != nil {
				t.Fatalf("add edge %s -> %s: %v\n%s", edge[0], edge[1], err, output)
			}
		}
		if output, err := runFlow(t, harness, "ticket", ids["D"], "#9"); err != nil {
			t.Fatalf("set D ticket: %v\n%s", err, output)
		}

		output, err := runFlow(t, harness, "status", "cyc", "--force")
		if err != nil {
			t.Fatalf("status cycle: %v\n%s", err, output)
		}
		if !strings.Contains(output, "[#9] X") {
			t.Fatalf("status cycle = %q, want X to inherit #9 after A is resolved", output)
		}
		return
	}
	t.Fatal("could not build a cycle fixture with B ordered before D")
}

func newCycleHarness(t *testing.T, attempt int) *testharness.Harness {
	t.Helper()
	return testharness.NewHarness(t, "cycle-"+string(rune('a'+attempt%26)), "session")
}
