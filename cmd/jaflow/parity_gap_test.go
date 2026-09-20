package main

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
	"github.com/jacazul-ai/flow/internal/testharness"
)

var shortTaskIDPattern = regexp.MustCompile(`Created task ([0-9a-f]{8})`)

func createdTaskIDs(output string) []string {
	matches := shortTaskIDPattern.FindAllStringSubmatch(output, -1)
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match[1])
	}
	return ids
}

func TestReferenceCommandSurfaceIsRoutable(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	for _, command := range []string{
		"next",
		"amend",
		"rename",
		"urgent",
		"block",
		"unblock",
		"wait",
		"cache",
	} {
		t.Run(command, func(t *testing.T) {
			output, err := runJaflow(t, binary, harness, "help", command)
			if err != nil {
				t.Fatalf("help %s failed: %v\n%s", command, err, output)
			}
			if !strings.Contains(output, "USAGE") {
				t.Fatalf("help %s = %q, want usage", command, output)
			}
		})
	}
}

func TestReferenceInitiativeAliasesRemainAvailable(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	for _, alias := range []string{"initiative", "ini"} {
		output, err := runJaflow(t, binary, harness, alias, alias+"-plan", "Task")
		if err != nil {
			t.Fatalf("create plan through %s: %v\n%s", alias, err, output)
		}
	}

	output, err := runJaflow(t, binary, harness, "inis", "--all")
	if err != nil {
		t.Fatalf("list plans through inis: %v\n%s", err, output)
	}
	for _, plan := range []string{"initiative-plan", "ini-plan"} {
		if !strings.Contains(output, plan) {
			t.Fatalf("inis output = %q, want %s", output, plan)
		}
	}

	output, err = runJaflow(t, binary, harness, "initiatives", "--all", "--force")
	if err != nil {
		t.Fatalf("list plans through initiatives: %v\n%s", err, output)
	}
	if !strings.Contains(output, "initiative-plan") || !strings.Contains(output, "ini-plan") {
		t.Fatalf("initiatives output = %q, want both aliases", output)
	}
}

func TestNextUsesReadyTasksWithinInitiative(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "parity", "First", "Second")
	if err != nil {
		t.Fatalf("create parity plan: %v\n%s", err, output)
	}
	ids := createdTaskIDs(output)
	if len(ids) != 2 {
		t.Fatalf("plan output = %q, want two task IDs", output)
	}

	output, err = runJaflow(t, binary, harness, "next", "parity")
	if err != nil {
		t.Fatalf("find first ready task: %v\n%s", err, output)
	}
	if !strings.Contains(output, "First") || strings.Contains(output, "Second") {
		t.Fatalf("first next output = %q, want only First", output)
	}

	for _, args := range [][]string{
		{"execute", ids[0]},
		{"outcome", ids[0], "First is complete"},
		{"done", ids[0]},
	} {
		if output, err := runJaflow(t, binary, harness, args...); err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
	}

	output, err = runJaflow(t, binary, harness, "next", "parity")
	if err != nil {
		t.Fatalf("find second ready task: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Second") {
		t.Fatalf("second next output = %q, want Second", output)
	}
}

func TestAmendAndRenamePreserveInitiativeMetadata(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "old-initiative", "Original")
	if err != nil {
		t.Fatalf("create metadata plan: %v\n%s", err, output)
	}
	ids := createdTaskIDs(output)
	if len(ids) != 1 {
		t.Fatalf("plan output = %q, want one task ID", output)
	}

	output, err = runJaflow(
		t,
		binary,
		harness,
		"amend",
		ids[0],
		"description=Updated",
		"ticket=#JAF-123",
	)
	if err != nil {
		t.Fatalf("amend task metadata: %v\n%s", err, output)
	}

	output, err = runJaflow(t, binary, harness, "rename", "old-initiative", "new-initiative")
	if err != nil {
		t.Fatalf("rename initiative: %v\n%s", err, output)
	}

	output, err = runJaflow(t, binary, harness, "status", "new-initiative", "--force")
	if err != nil {
		t.Fatalf("read renamed initiative: %v\n%s", err, output)
	}
	for _, expected := range []string{"Updated", "#JAF-123"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("renamed status = %q, want %s", output, expected)
		}
	}
}

func TestFocusInterestAndCacheControlsAreAvailable(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	if output, err := runJaflow(t, binary, harness, "plan", "cache", "Task"); err != nil {
		t.Fatalf("create cache plan: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "focus", "plan", "cache"); err != nil {
		t.Fatalf("focus cache plan: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "focus", "interest", "add", "cache"); err != nil {
		t.Fatalf("add plan interest: %v\n%s", err, output)
	}

	output, err := runJaflow(t, binary, harness, "focus", "interest", "list")
	if err != nil {
		t.Fatalf("list plan interests: %v\n%s", err, output)
	}
	if !strings.Contains(output, "cache") {
		t.Fatalf("interest list = %q, want cache initiative", output)
	}

	if output, err := runJaflow(t, binary, harness, "status", "cache"); err != nil {
		t.Fatalf("prime status cache: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "cache", "info")
	if err != nil || !strings.Contains(output, "file") {
		t.Fatalf("cache info = %q, err %v; want file count", output, err)
	}

	if output, err := runJaflow(t, binary, harness, "cache", "clear", "status"); err != nil {
		t.Fatalf("clear status cache: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "status", "cache")
	if err != nil || strings.Contains(output, "[cached]") {
		t.Fatalf("status after cache clear = %q, err %v; want fresh output", output, err)
	}
}

func TestDependencyCommandsPreserveReadySelection(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "dependencies", "First", "Second")
	if err != nil {
		t.Fatalf("create dependency plan: %v\n%s", err, output)
	}
	ids := createdTaskIDs(output)
	if len(ids) != 2 {
		t.Fatalf("plan output = %q, want two task IDs", output)
	}

	if output, err := runJaflow(t, binary, harness, "unblock", ids[1], ids[0]); err != nil {
		t.Fatalf("remove dependency: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "next", "dependencies")
	if err != nil {
		t.Fatalf("list unblocked tasks: %v\n%s", err, output)
	}
	if !strings.Contains(output, "First") || !strings.Contains(output, "Second") {
		t.Fatalf("unblocked next output = %q, want both tasks", output)
	}

	if output, err := runJaflow(t, binary, harness, "block", ids[1], ids[0]); err != nil {
		t.Fatalf("add dependency: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "next", "dependencies")
	if err != nil {
		t.Fatalf("list blocked tasks: %v\n%s", err, output)
	}
	if !strings.Contains(output, "First") || strings.Contains(output, "Second") {
		t.Fatalf("blocked next output = %q, want only First", output)
	}
}

func TestDashboardIncludesSessionAndRecentlyClosedContext(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "dashboard", "Active")
	if err != nil {
		t.Fatalf("create dashboard plan: %v\n%s", err, output)
	}
	activeIDs := createdTaskIDs(output)
	if len(activeIDs) != 1 {
		t.Fatalf("dashboard plan output = %q, want one task ID", output)
	}
	for _, args := range [][]string{
		{"focus", "task", activeIDs[0]},
		{"focus", "interest", "add", "dashboard"},
	} {
		if output, err := runJaflow(t, binary, harness, args...); err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
	}

	output, err = runJaflow(t, binary, harness, "ponder", "--force")
	if err != nil {
		t.Fatalf("render active dashboard: %v\n%s", err, output)
	}
	for _, expected := range []string{
		"SESSION CONTEXT",
		"Interests: dashboard",
		"[PULSE SUMMARY]",
		"[TASK LANDSCAPE]",
		"[TACTICAL READOUT]",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("dashboard output = %q, want %s", output, expected)
		}
	}

	output, err = runJaflow(t, binary, harness, "plan", "closed", "Closed task")
	if err != nil {
		t.Fatalf("create closed plan: %v\n%s", err, output)
	}
	closedIDs := createdTaskIDs(output)
	if len(closedIDs) != 1 {
		t.Fatalf("closed plan output = %q, want one task ID", output)
	}
	for _, args := range [][]string{
		{"execute", closedIDs[0]},
		{"outcome", closedIDs[0], "Closed outcome"},
		{"done", closedIDs[0]},
	} {
		if output, err := runJaflow(t, binary, harness, args...); err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
	}

	output, err = runJaflow(t, binary, harness, "ponder", "--force")
	if err != nil {
		t.Fatalf("render dashboard history: %v\n%s", err, output)
	}
	for _, expected := range []string{"[RECENTLY CLOSED]", "closed", "Closed outcome"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("history output = %q, want %s", output, expected)
		}
	}
}

func TestRoadmapSupportsReferenceAddAndShipAliases(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	if output, err := runJaflow(t, binary, harness, "plan", "roadmap-target", "Task"); err != nil {
		t.Fatalf("create roadmap target: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "roadmap", "init"); err != nil {
		t.Fatalf("initialize roadmap: %v\n%s", err, output)
	}
	output, err := runJaflow(t, binary, harness, "roadmap", "add", "next", "Positional phase")
	if err != nil {
		t.Fatalf("add positional roadmap phase: %v\n%s", err, output)
	}
	match := regexp.MustCompile(`Roadmap phase added: \[next\] Positional phase \(([^)]+)\)`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("roadmap add output = %q, want entry ID", output)
	}

	output, err = runJaflow(t, binary, harness, "ship", match[1])
	if err != nil {
		t.Fatalf("ship roadmap phase through alias: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "roadmap", "show")
	if err != nil || !strings.Contains(output, "[shipped] Positional phase") {
		t.Fatalf("roadmap after alias ship = %q, err %v; want shipped phase", output, err)
	}
}

func TestPlansPreserveFiltersAndListOutput(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	for _, plan := range []string{"open-plan", "closed-plan"} {
		if output, err := runJaflow(t, binary, harness, "plan", plan, "Task"); err != nil {
			t.Fatalf("create %s: %v\n%s", plan, err, output)
		}
	}
	planOutput, err := runJaflow(t, binary, harness, "plan", "closed-plan-2", "Task")
	if err != nil {
		t.Fatalf("create second closed plan: %v\n%s", err, planOutput)
	}
	ids := createdTaskIDs(planOutput)
	if len(ids) != 1 {
		t.Fatalf("closed plan output = %q, want one task ID", planOutput)
	}
	for _, args := range [][]string{
		{"execute", ids[0]},
		{"outcome", ids[0], "Closed"},
		{"done", ids[0]},
	} {
		if output, err := runJaflow(t, binary, harness, args...); err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
	}

	output, err := runJaflow(t, binary, harness, "plans", "--force")
	if err != nil {
		t.Fatalf("list active plans: %v\n%s", err, output)
	}
	if !strings.Contains(output, "open-plan") || strings.Contains(output, "closed-plan-2") || strings.Contains(output, "SESSION CONTEXT") {
		t.Fatalf("active plans output = %q, want list-only active output", output)
	}

	output, err = runJaflow(t, binary, harness, "plans", "--closed", "--force")
	if err != nil {
		t.Fatalf("list closed plans: %v\n%s", err, output)
	}
	if !strings.Contains(output, "closed-plan-2") || strings.Contains(output, "open-plan") {
		t.Fatalf("closed plans output = %q, want only closed plan", output)
	}

	output, err = runJaflow(t, binary, harness, "plans", "--all", "--force")
	if err != nil {
		t.Fatalf("list all plans: %v\n%s", err, output)
	}
	if !strings.Contains(output, "open-plan") || !strings.Contains(output, "closed-plan-2") {
		t.Fatalf("all plans output = %q, want both plans", output)
	}
}

func TestFocusCompatibilityAliasesAndDefaultShow(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	if output, err := runJaflow(t, binary, harness, "plan", "focus-alias", "Task"); err != nil {
		t.Fatalf("create focus alias plan: %v\n%s", err, output)
	}
	for _, args := range [][]string{
		{"focus", "ini", "focus-alias"},
		{"focus", "focus-alias"},
	} {
		if output, err := runJaflow(t, binary, harness, args...); err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
	}
	output, err := runJaflow(t, binary, harness, "focus")
	if err != nil {
		t.Fatalf("show default focus: %v\n%s", err, output)
	}
	for _, expected := range []string{"FOCUS", "focus-alias", "Task:"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("default focus output = %q, want %s", output, expected)
		}
	}
}

func TestPlansCacheInvalidatesAfterInitiativeCreation(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	if output, err := runJaflow(t, binary, harness, "plan", "first-plan", "First"); err != nil {
		t.Fatalf("create first plan: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "plans"); err != nil {
		t.Fatalf("prime plans cache: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "plan", "second-plan", "Second"); err != nil {
		t.Fatalf("create second plan: %v\n%s", err, output)
	}
	output, err := runJaflow(t, binary, harness, "plans")
	if err != nil {
		t.Fatalf("read refreshed plans: %v\n%s", err, output)
	}
	if strings.Contains(output, "[cached]") || !strings.Contains(output, "second-plan") {
		t.Fatalf("plans after creation = %q, want refreshed second plan", output)
	}
}

func TestPonderTableOutputMatchesReferenceColumns(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	if output, err := runJaflow(t, binary, harness, "plan", "table-plan", "Task"); err != nil {
		t.Fatalf("create table plan: %v\n%s", err, output)
	}
	output, err := runJaflow(t, binary, harness, "ponder", "--table", "--force")
	if err != nil {
		t.Fatalf("render table dashboard: %v\n%s", err, output)
	}
	if !strings.Contains(output, "| ST | UUID | MODE | PLAN | DESCRIPTION | URG |") {
		t.Fatalf("table dashboard = %q, want reference columns", output)
	}
}

func TestFocusPlanNeverSelectsBlockedTask(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	ctx := context.Background()
	store, err := sqlite.Open(ctx, harness.DatabasePath)
	if err != nil {
		t.Fatalf("open focus safety database: %v", err)
	}
	initiative, err := store.GetOrCreateInitiative(ctx, task.CreateInitiativeInput{
		ProjectID: harness.ProjectID,
		Name:      "focus-safety",
	})
	if err != nil {
		store.Close()
		t.Fatalf("create focus safety initiative: %v", err)
	}
	create := func(description string, dependencies ...string) task.Task {
		t.Helper()
		created, err := store.CreateTask(ctx, task.CreateTaskInput{
			InitiativeID: initiative.ID,
			Description:  description,
			Dependencies: dependencies,
		})
		if err != nil {
			t.Fatalf("create %s: %v", description, err)
		}
		return created
	}
	active := create("Active task")
	blocking := create("Blocking task")
	blocked := create("Blocked task", blocking.ID)
	ready := create("Ready task")
	for _, current := range []task.Task{active, blocking} {
		if err := store.StartTask(ctx, current.ID); err != nil {
			store.Close()
			t.Fatalf("start %s: %v", current.Description, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close focus safety database: %v", err)
	}

	output, err := runJaflow(t, binary, harness, "focus", "plan", "focus-safety")
	if err != nil {
		t.Fatalf("focus safety plan: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Next task "+shortIDForTest(ready.ID)) {
		t.Fatalf("focus output = %q, want ready task", output)
	}
	if strings.Contains(output, shortIDForTest(blocked.ID)) {
		t.Fatalf("focus output selected blocked task: %q", output)
	}
}

func shortIDForTest(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
