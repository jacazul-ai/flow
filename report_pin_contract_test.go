package flow_test

import (
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

// TestReportOutputOnChainedFixture is a characterization test: it pins the
// exact output of the report commands before their query pattern changes, and
// the golden stays frozen during that refactor (docs/characterization-tests.md).
// The fixture covers a ticket at the head of a chain overridden by a nearer
// one mid-chain (alpha), a chain with no ticket (beta), a mid-chain ticket
// below an untagged head (gamma), completed and active tasks, and pending
// positions interleaved across initiatives.
func TestReportOutputOnChainedFixture(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")
	ids := buildChainedReportFixture(t, harness)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		ids[initiativeIDForTest(t, harness, name)[:8]] = "<" + name + ">"
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{
			args: []string{"status", "--force"},
			want: `══ Plan: ALL ACTIVE ══
PENDING:
- [<G1>] G1
- [<A2>] [#ROOT-1] A2
- [<B2>] B2
- [<G2>] [#MID-2] G2
- [<A3>] [#MID-3] A3
- [<B3>] B3
- [<G3>] [#MID-2] G3
- [<A4>] [#MID-3] A4
COMPLETED:
- [<A1>] [#ROOT-1] A1
- [<B1>] B1
`,
		},
		{
			args: []string{"status", "alpha", "--force"},
			want: `══ Plan: alpha ══
PENDING:
- [<A2>] [#ROOT-1] A2
- [<A3>] [#MID-3] A3
- [<A4>] [#MID-3] A4
COMPLETED:
- [<A1>] [#ROOT-1] A1
`,
		},
		{
			args: []string{"status", "gamma", "--force"},
			want: `══ Plan: gamma ══
PENDING:
- [<G1>] G1
- [<G2>] [#MID-2] G2
- [<G3>] [#MID-2] G3
`,
		},
		{
			args: []string{"plans", "--force"},
			want: `PROJECT: project
INITIATIVES:
- [ACTIVE] alpha [id:<alpha>] pending:3 active:0 completed:1 blocked:2
- [ACTIVE] beta [id:<beta>] pending:2 active:0 completed:1 blocked:1
- [ACTIVE] gamma [id:<gamma>] pending:2 active:1 completed:0 blocked:2
`,
		},
		{
			args: []string{"ponder", "--force"},
			want: `PROJECT: project
INITIATIVES:
- [ACTIVE] alpha pending:3 active:0 completed:1 blocked:2
- [ACTIVE] beta pending:2 active:0 completed:1 blocked:1
- [ACTIVE] gamma pending:2 active:1 completed:0 blocked:2
SESSION CONTEXT:
  Focus: None | Task: None

[PULSE SUMMARY]
  Focused: None | Plans: 3 | Done Today: 2
  Health | Pending: 8 | Active: 1 | Overdue: 0
  Registry | Initiatives: 3

[TASK LANDSCAPE]
  alpha | Active: 0 | Ready: 1 | Total: 4
  beta | Active: 0 | Ready: 1 | Total: 3
  gamma | Active: 1 | Ready: 0 | Total: 3

[TACTICAL READOUT]
- [ACTIVE] <G1> | UNSPECIFIED | gamma | G1 | [0.0]
- [PENDING] <A2> | UNSPECIFIED | alpha | A2 | [0.0]
- [PENDING] <B2> | UNSPECIFIED | beta | B2 | [0.0]
- [PENDING] <G2> | UNSPECIFIED | gamma | G2 | [0.0]
- [PENDING] <A3> | UNSPECIFIED | alpha | A3 | [0.0]
- [PENDING] <B3> | UNSPECIFIED | beta | B3 | [0.0]
- [PENDING] <G3> | UNSPECIFIED | gamma | G3 | [0.0]
- [PENDING] <A4> | UNSPECIFIED | alpha | A4 | [0.0]

`,
		},
		{
			args: []string{"next"},
			want: "<A2> A2\n<B2> B2\n",
		},
		{
			args: []string{"next", "alpha"},
			want: "<A2> A2\n",
		},
	} {
		output, err := runFlow(t, harness, tc.args...)
		if err != nil {
			t.Fatalf("run %v: %v\n%s", tc.args, err, output)
		}
		if got := replaceIDs(output, ids); got != tc.want {
			t.Errorf("%v output changed\n--- got\n%s--- want\n%s", tc.args, got, tc.want)
		}
	}
}

// buildChainedReportFixture creates the fixture through the real CLI and
// returns each task's short UUID mapped to a <description> placeholder.
func buildChainedReportFixture(t *testing.T, harness *testharness.Harness) map[string]string {
	t.Helper()

	ids := make(map[string]string)
	byName := make(map[string]string)
	for _, plan := range [][]string{
		{"alpha", "A1", "A2", "A3", "A4"},
		{"beta", "B1", "B2", "B3"},
		{"gamma", "G1", "G2", "G3"},
	} {
		output, err := runFlow(t, harness, append([]string{"plan"}, plan...)...)
		if err != nil {
			t.Fatalf("create %s plan: %v\n%s", plan[0], err, output)
		}
		created := createdTaskIDs(output)
		if len(created) != len(plan)-1 {
			t.Fatalf("%s plan output = %q, want %d task IDs", plan[0], output, len(plan)-1)
		}
		for i, id := range created {
			ids[id] = "<" + plan[i+1] + ">"
			byName[plan[i+1]] = id
		}
	}

	for _, args := range [][]string{
		{"ticket", byName["A1"], "#ROOT-1"},
		{"ticket", byName["A3"], "#MID-3"},
		{"ticket", byName["G2"], "#MID-2"},
		{"execute", byName["A1"]},
		{"outcome", byName["A1"], "A1 is complete"},
		{"done", byName["A1"]},
		{"execute", byName["B1"]},
		{"outcome", byName["B1"], "B1 is complete"},
		{"done", byName["B1"]},
		{"execute", byName["G1"]},
	} {
		if output, err := runFlow(t, harness, args...); err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
	}
	return ids
}

func replaceIDs(output string, ids map[string]string) string {
	for id, placeholder := range ids {
		output = strings.ReplaceAll(output, id, placeholder)
	}
	return output
}
