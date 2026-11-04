package flow_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

// migratedPonderSource is a legacy Taskwarrior export shaped like a real
// tw-flow project: sixteen tagged pending tasks, legacy [MODE] prefixes,
// archive and trash projects, and four plans closed out of alphabetical order
// with their OUTCOME annotations.
func migratedPonderSource() string {
	var records []string
	for index := 1; index <= 16; index++ {
		records = append(records, fmt.Sprintf(`{"uuid": "00000000-0000-4000-8000-%012d", "project": "busy",
  "description": "[EXECUTE] Busy task %02d", "status": "pending", "entry": "20261101T120000Z",
  "tags": ["implementation"]}`, index, index))
	}
	records = append(records,
		`{"uuid": "10000000-0000-4000-8000-000000000001", "project": "prefixed",
  "description": "[PLAN] Map the cutover", "status": "pending", "entry": "20261101T120000Z", "tags": ["next"]}`,
		`{"uuid": "10000000-0000-4000-8000-000000000002", "project": "busy:_archive",
  "description": "Archived task", "status": "pending", "entry": "20261101T120000Z"}`,
		`{"uuid": "10000000-0000-4000-8000-000000000003", "project": "old:_trash",
  "description": "Trashed task", "status": "pending", "entry": "20261101T120000Z"}`,
	)
	for index, plan := range []struct{ name, end string }{
		{"c-closed", "20261028T120000Z"},
		{"a-closed", "20261031T120000Z"},
		{"d-closed", "20261101T120000Z"},
		{"b-closed", "20261102T120000Z"},
	} {
		records = append(records, fmt.Sprintf(`{"uuid": "20000000-0000-4000-8000-%012d", "project": %q,
  "description": "Close %s", "status": "completed", "entry": "20261020T120000Z", "end": %q,
  "annotations": [{"entry": %q, "description": "OUTCOME: Shipped %s"}]}`,
			index, plan.name, plan.name, plan.end, plan.end, plan.name))
	}
	return "[\n" + strings.Join(records, ",\n") + "\n]"
}

func TestPonderOverMigratedDataMatchesTheReferenceShape(t *testing.T) {
	harness := testharness.NewHarness(t, "project", "session")
	foreign := *harness
	foreign.ProjectID = "foreign"
	source := harness.WriteFile(t, "source.json", []byte(migratedPonderSource()))
	mustFlow(t, harness, "migrate", "taskwarrior", "--source", source, "--apply")
	mustFlow(t, &foreign, "plan", "foreign-plan", "Foreign task")

	output := mustFlow(t, harness, "ponder", "--force")
	if !strings.Contains(output, "Global | Pending: 19 | Active: 0 | Registry: 1 Arch / 1 Trash") {
		t.Fatalf("ponder = %q, want the migrated registry", output)
	}

	readout := section(t, output, "TACTICAL READOUT")
	if len(readout) != 15 {
		t.Fatalf("readout = %d lines, want 15", len(readout))
	}
	if !regexp.MustCompile(`\| PLAN \| prefixed \| \+next \| Map the cutover \|`).MatchString(readout[0]) {
		t.Fatalf("readout first line = %q, want the +next [PLAN] task on top", readout[0])
	}
	if !regexp.MustCompile(`\| EXECUTE \| busy \| \+implementation \| Busy task \d\d \|`).MatchString(readout[1]) {
		t.Fatalf("readout line = %q, want the legacy EXECUTE prefix as mode", readout[1])
	}

	closed := section(t, output, "RECENTLY CLOSED")
	want := []string{
		"  ✓ b-closed [2026-11-02] — Shipped b-closed",
		"  ✓ d-closed [2026-11-01] — Shipped d-closed",
		"  ✓ a-closed [2026-10-31] — Shipped a-closed",
	}
	if strings.Join(closed, "\n") != strings.Join(want, "\n") {
		t.Fatalf("recently closed = %q, want %q", closed, want)
	}

	landscape := strings.Join(section(t, output, "TASK LANDSCAPE"), "\n")
	if strings.Contains(landscape, "closed") || strings.Contains(landscape, "old") || !strings.Contains(landscape, "busy | Active: 0 | Ready: 16 | Total: 16") {
		t.Fatalf("landscape = %q, want only the plans with open active-bucket tasks", landscape)
	}

	if foreignOutput := mustFlow(t, &foreign, "ponder", "--force"); strings.Contains(foreignOutput, "busy") || strings.Contains(foreignOutput, "closed") {
		t.Fatalf("foreign ponder = %q, leaked migrated data", foreignOutput)
	}
	if strings.Contains(output, "foreign-plan") {
		t.Fatalf("ponder = %q, leaked the foreign project", output)
	}
}
