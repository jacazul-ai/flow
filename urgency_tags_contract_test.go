package flow_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

// urgencyOf reads the derived urgency of one task through its initiative's
// structured status.
func urgencyOf(t *testing.T, harness *testharness.Harness, initiative string) float64 {
	t.Helper()
	var report struct {
		Records []struct {
			Urgency float64 `json:"urgency"`
		} `json:"records"`
	}
	output := mustFlow(t, harness, "status", initiative, "--format", "json", "--force")
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode %s status: %v\n%s", initiative, err, output)
	}
	if len(report.Records) != 1 {
		t.Fatalf("%s status = %d records, want 1", initiative, len(report.Records))
	}
	return report.Records[0].Urgency
}

// Taskwarrior adds 1.0 x a count factor (0.8, 0.9, 1.0) for tags, and the
// Jacazul taskrc adds next=10.0 and critical=5.0 on top of that term.
func TestUrgencyCountsTagsLikeTheReferenceTaskrc(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")
	foreign := *harness
	foreign.ProjectID = "foreign"
	cases := []struct {
		initiative string
		tags       []string
		delta      float64
	}{
		{"untagged", nil, 0},
		{"one-tag", []string{"+research"}, 0.8},
		{"two-tags", []string{"+research", "+e2e"}, 0.9},
		{"three-tags", []string{"+research", "+e2e", "+parity"}, 1.0},
		{"next-tag", []string{"+next"}, 10.8},
		{"critical-tag", []string{"+critical"}, 5.8},
		{"both-tags", []string{"+next", "+critical"}, 15.9},
	}
	for _, tc := range cases {
		ids := createdTaskIDs(mustFlow(t, harness, "plan", tc.initiative, "Task of "+tc.initiative))
		if len(tc.tags) > 0 {
			mustFlow(t, harness, append([]string{"amend", ids[0]}, tc.tags...)...)
		}
	}
	ids := createdTaskIDs(mustFlow(t, &foreign, "plan", "foreign-plan", "Foreign task"))
	mustFlow(t, &foreign, "amend", ids[0], "+next")

	base := urgencyOf(t, harness, "untagged")
	for _, tc := range cases {
		got := urgencyOf(t, harness, tc.initiative) - base
		if math.Abs(got-tc.delta) > 0.001 {
			t.Fatalf("%s urgency delta = %.3f, want %.3f", tc.initiative, got, tc.delta)
		}
	}
	if got := urgencyOf(t, &foreign, "foreign-plan") - base; math.Abs(got-10.8) > 0.001 {
		t.Fatalf("foreign next-tag delta = %.3f, want 10.8 within its own project", got)
	}
}
