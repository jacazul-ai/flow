package flow_test

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

const longOutcome = "Closed with an outcome long enough to be cut by the dashboard at sixty characters"

// ponderFixture builds a migration-sized project: more than fifteen pending
// tasks, more than three closed plans closed out of alphabetical order, an
// archived and a trashed plan, a description with a legacy [PLAN] prefix, and
// a foreign project in the same database.
type ponderFixture struct {
	harness *testharness.Harness
	foreign *testharness.Harness
	active  string
}

func newPonderFixture(t *testing.T) ponderFixture {
	t.Helper()
	harness := testharness.NewHarness(t, "project", "session")
	foreign := *harness
	foreign.ProjectID = "foreign"

	specs := []string{"plan", "busy"}
	for index := 1; index <= 16; index++ {
		specs = append(specs, fmt.Sprintf("Busy task %02d", index))
	}
	busy := createdTaskIDs(mustFlow(t, harness, specs...))
	if len(busy) != 16 {
		t.Fatalf("busy plan = %d tasks, want 16", len(busy))
	}
	mustFlow(t, harness, "execute", busy[0])
	mustFlow(t, harness, "plan", "prefixed", "[PLAN] Map the cutover")

	for _, name := range []string{"c-closed", "a-closed", "d-closed", "b-closed"} {
		ids := createdTaskIDs(mustFlow(t, harness, "plan", name, "Close "+name))
		mustFlow(t, harness, "execute", ids[0])
		mustFlow(t, harness, "outcome", ids[0], longOutcome+" "+name)
		mustFlow(t, harness, "done", ids[0])
	}
	archived := createdTaskIDs(mustFlow(t, harness, "plan", "archived-plan", "Archived task"))
	trashed := createdTaskIDs(mustFlow(t, harness, "plan", "trashed-plan", "Trashed task"))
	db, err := sql.Open("sqlite", harness.DatabasePath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	for id, bucket := range map[string]string{archived[0]: "archive", trashed[0]: "trash"} {
		if _, err := db.Exec("UPDATE tasks SET bucket = ? WHERE id LIKE ?", bucket, id+"%"); err != nil {
			t.Fatalf("move %s to %s: %v", id, bucket, err)
		}
	}
	db.Close()

	mustFlow(t, &foreign, "plan", "foreign-plan", "Foreign task")
	return ponderFixture{harness: harness, foreign: &foreign, active: busy[0]}
}

func section(t *testing.T, output string, name string) []string {
	t.Helper()
	start := strings.Index(output, "["+name+"]\n")
	if start < 0 {
		t.Fatalf("output has no [%s] section:\n%s", name, output)
	}
	body := output[start+len(name)+3:]
	if end := strings.Index(body, "\n\n"); end >= 0 {
		body = body[:end]
	}
	return strings.Split(strings.TrimRight(body, "\n"), "\n")
}

func TestPonderRendersTheReferenceDashboardShape(t *testing.T) {
	fx := newPonderFixture(t)
	output := mustFlow(t, fx.harness, "ponder", "--force")

	if strings.Contains(output, "INITIATIVES:") {
		t.Fatalf("ponder = %q, want no initiative list", output)
	}
	if got := strings.Count(output, "SESSION CONTEXT"); got != 1 {
		t.Fatalf("ponder session context = %d, want 1", got)
	}
	if !strings.Contains(output, "Global | Pending: 19 | Active: 1 | Registry: 1 Arch / 1 Trash") {
		t.Fatalf("ponder = %q, want the global pulse line", output)
	}

	landscape := strings.Join(section(t, output, "TASK LANDSCAPE"), "\n")
	for _, name := range []string{"busy", "prefixed"} {
		if !strings.Contains(landscape, name) {
			t.Fatalf("landscape = %q, want %s", landscape, name)
		}
	}
	for _, name := range []string{"closed", "archived-plan", "trashed-plan", "foreign-plan"} {
		if strings.Contains(landscape, name) {
			t.Fatalf("landscape = %q, want no %s", landscape, name)
		}
	}
	if !regexp.MustCompile(`busy \| Active: 1 \| Ready: 0 \| Total: 16`).MatchString(landscape) {
		t.Fatalf("landscape = %q, want pending-only totals", landscape)
	}

	readout := section(t, output, "TACTICAL READOUT")
	if len(readout) != 15 {
		t.Fatalf("readout = %d lines, want 15:\n%s", len(readout), strings.Join(readout, "\n"))
	}
	if !strings.Contains(readout[0], fx.active) {
		t.Fatalf("readout first line = %q, want the active task", readout[0])
	}
	for _, line := range readout {
		if strings.Contains(line, "Archived task") || strings.Contains(line, "Trashed task") || strings.Contains(line, "Foreign task") {
			t.Fatalf("readout line %q leaked a bucketed or foreign task", line)
		}
	}

	closed := section(t, output, "RECENTLY CLOSED")
	if len(closed) != 3 {
		t.Fatalf("recently closed = %q, want 3 plans", closed)
	}
	for index, name := range []string{"b-closed", "d-closed", "a-closed"} {
		pattern := regexp.MustCompile(`^  ✓ ` + name + ` \[\d{4}-\d{2}-\d{2}\] — (.+)$`)
		match := pattern.FindStringSubmatch(closed[index])
		if match == nil {
			t.Fatalf("recently closed line %d = %q, want %s with its date", index, closed[index], name)
		}
		if got := len([]rune(match[1])); got != 60 {
			t.Fatalf("recently closed outcome = %q (%d runes), want 60", match[1], got)
		}
	}
}

func TestPonderShowsTheLegacyModePrefix(t *testing.T) {
	fx := newPonderFixture(t)
	mustFlow(t, fx.harness, "focus", "interest", "add", "prefixed")
	readout := section(t, mustFlow(t, fx.harness, "ponder", "--force"), "TACTICAL READOUT")
	if len(readout) != 1 {
		t.Fatalf("readout = %q, want the prefixed task only", readout)
	}
	if !strings.Contains(readout[0], "| PLAN | prefixed |") || strings.Contains(readout[0], "[PLAN]") || !strings.Contains(readout[0], "Map the cutover") {
		t.Fatalf("readout = %q, want mode PLAN and the description without its prefix", readout[0])
	}
}

func TestPonderLandscapeSkipsInterestingPlansWithoutPendingTasks(t *testing.T) {
	fx := newPonderFixture(t)
	mustFlow(t, fx.harness, "focus", "interest", "add", "a-closed")
	mustFlow(t, fx.harness, "focus", "interest", "add", "busy")
	output := mustFlow(t, fx.harness, "ponder", "--force")
	landscape := strings.Join(section(t, output, "TASK LANDSCAPE"), "\n")
	if strings.Contains(landscape, "a-closed") || !strings.Contains(landscape, "busy") {
		t.Fatalf("landscape = %q, want busy only", landscape)
	}
	if !strings.Contains(output, "Health | Pending: 16 | Active: 1") {
		t.Fatalf("ponder = %q, want health over the interesting plans", output)
	}
}

func TestOnboardWithoutFocusRendersSessionContextOnce(t *testing.T) {
	fx := newPonderFixture(t)
	output := mustFlow(t, fx.harness, "onboard")
	if got := strings.Count(output, "SESSION CONTEXT"); got != 1 {
		t.Fatalf("onboard session context = %d, want 1:\n%s", got, output)
	}
	if len(section(t, output, "TACTICAL READOUT")) != 15 || len(section(t, output, "RECENTLY CLOSED")) != 3 {
		t.Fatalf("onboard = %q, want the reference dashboard limits", output)
	}
}

func TestPonderStaysProjectIsolated(t *testing.T) {
	fx := newPonderFixture(t)
	output := mustFlow(t, fx.foreign, "ponder", "--force")
	for _, leak := range []string{"busy", "closed", "prefixed", "archived-plan"} {
		if strings.Contains(output, leak) {
			t.Fatalf("foreign ponder = %q, leaked %s", output, leak)
		}
	}
	if !strings.Contains(output, "foreign-plan") || !strings.Contains(output, "Global | Pending: 1 | Active: 0 | Registry: 0 Arch / 0 Trash") {
		t.Fatalf("foreign ponder = %q, want only its own plan", output)
	}
}
