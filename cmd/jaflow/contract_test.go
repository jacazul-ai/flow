package main

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

func buildJaflow(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "jaflow")
	command := exec.Command("go", "build", "-o", binary, "./cmd/jaflow")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build jaflow: %v\n%s", err, output)
	}
	return binary
}

func runJaflow(t *testing.T, binary string, harness *testharness.Harness, args ...string) (string, error) {
	t.Helper()

	command := exec.Command(binary, args...)
	command.Dir = harness.Root
	command.Env = harness.Environment
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestPlanStateIsIsolatedByProject(t *testing.T) {
	binary := buildJaflow(t)
	first := testharness.NewHarness(t, "project-alpha", "session-alpha")
	second := testharness.NewHarness(t, "project-beta", "session-beta")

	firstOutput, err := runJaflow(t, binary, first, "plan", "alpha", "Alpha task")
	if err != nil {
		t.Fatalf("create alpha plan: %v\n%s", err, firstOutput)
	}
	if !strings.Contains(firstOutput, "Alpha task") {
		t.Fatalf("alpha output = %q, want task description", firstOutput)
	}

	secondOutput, err := runJaflow(t, binary, second, "plan", "beta", "Beta task")
	if err != nil {
		t.Fatalf("create beta plan: %v\n%s", err, secondOutput)
	}
	if !strings.Contains(secondOutput, "Beta task") || strings.Contains(secondOutput, "Alpha task") {
		t.Fatalf("beta output crossed project boundary: %q", secondOutput)
	}

	firstOutput, err = runJaflow(t, binary, first, "status")
	if err != nil {
		t.Fatalf("read alpha status: %v\n%s", err, firstOutput)
	}
	if !strings.Contains(firstOutput, "Alpha task") || strings.Contains(firstOutput, "Beta task") {
		t.Fatalf("alpha output crossed project boundary: %q", firstOutput)
	}
}

func TestPlanCreationReturnsShortUUID(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "parity", "A task")
	if err != nil {
		t.Fatalf("create plan: %v\n%s", err, output)
	}
	if !regexp.MustCompile(`Created task [0-9a-f]{8}`).MatchString(output) {
		t.Fatalf("plan output = %q, want short UUID", output)
	}
}

func TestUnknownCommandReturnsActionableError(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "not-a-command")
	if err == nil {
		t.Fatalf("unknown command succeeded with output %q", output)
	}
	if !strings.Contains(strings.ToLower(output), "unknown") {
		t.Fatalf("unknown command output = %q, want actionable error", output)
	}
	if !strings.Contains(output, "ACTION:") {
		t.Fatalf("unknown command output = %q, want an ACTION prompt", output)
	}
}

func TestHelpProvidesAgentWorkflowBriefing(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	rootOutput, err := runJaflow(t, binary, harness, "help")
	if err != nil {
		t.Fatalf("root help failed: %v\n%s", err, rootOutput)
	}
	for _, section := range []string{"ROLE", "WORKFLOW", "COMMANDS", "AGENT RULES", "NEXT"} {
		if !strings.Contains(rootOutput, section) {
			t.Fatalf("root help = %q, want %q section", rootOutput, section)
		}
	}

	planOutput, err := runJaflow(t, binary, harness, "help", "plan")
	if err != nil {
		t.Fatalf("plan help failed: %v\n%s", err, planOutput)
	}
	for _, section := range []string{"PREREQUISITES", "SIDE EFFECTS AND OUTPUT", "EXAMPLES", "NEXT ACTION"} {
		if !strings.Contains(planOutput, section) {
			t.Fatalf("plan help = %q, want %q section", planOutput, section)
		}
	}
}

func TestRootHelpFlagsUseCustomRenderer(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	var explicitHelp string

	for _, args := range [][]string{{"help"}, {"--help"}} {
		output, err := runJaflow(t, binary, harness, args...)
		if err != nil {
			t.Fatalf("help %v failed: %v\n%s", args, err, output)
		}
		for _, section := range []string{"ROLE", "WORKFLOW", "COMMANDS", "GLOBAL OPTIONS"} {
			if !strings.Contains(output, section) {
				t.Fatalf("help %v = %q, want %q section", args, output, section)
			}
		}
		if strings.Contains(output, "Parser options:") || strings.Contains(output, "Available commands:") {
			t.Fatalf("help %v leaked go-flags output: %q", args, output)
		}
		if args[0] == "help" {
			explicitHelp = output
			continue
		}
		if output != explicitHelp {
			t.Fatalf("root help differs between help and --help\nhelp:\n%s\n--help:\n%s", explicitHelp, output)
		}
	}
}

func TestTaskLifecycleEnforcesOutcomeAndUnblocks(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	planOutput, err := runJaflow(t, binary, harness, "plan", "parity", "First", "Second")
	if err != nil {
		t.Fatalf("create plan: %v\n%s", err, planOutput)
	}
	matches := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindAllStringSubmatch(planOutput, -1)
	if len(matches) != 2 {
		t.Fatalf("plan output = %q, want two task UUIDs", planOutput)
	}
	first, second := matches[0][1], matches[1][1]

	if output, err := runJaflow(t, binary, harness, "execute", first); err != nil {
		t.Fatalf("execute first task: %v\n%s", err, output)
	}
	output, err := runJaflow(t, binary, harness, "done", first)
	if err == nil || !strings.Contains(output, "OUTCOME") {
		t.Fatalf("done without outcome = %q, err %v; want OUTCOME gate", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "outcome", first, "First is complete"); err != nil {
		t.Fatalf("record first outcome: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "done", first)
	if err != nil || !strings.Contains(output, "Ready task "+second) {
		t.Fatalf("complete first task = %q, err %v; want second ready", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "execute", second); err != nil {
		t.Fatalf("execute second task: %v\n%s", err, output)
	}
}

func TestFocusSwitchesTaskStack(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	planOutput, err := runJaflow(t, binary, harness, "plan", "focus", "First", "Second")
	if err != nil {
		t.Fatalf("create focus plan: %v\n%s", err, planOutput)
	}
	matches := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindAllStringSubmatch(planOutput, -1)
	if len(matches) != 2 {
		t.Fatalf("plan output = %q, want two task UUIDs", planOutput)
	}
	first, second := matches[0][1], matches[1][1]

	for _, taskID := range []string{first, second} {
		if output, err := runJaflow(t, binary, harness, "focus", "task", taskID); err != nil {
			t.Fatalf("focus task %s: %v\n%s", taskID, err, output)
		}
	}
	output, err := runJaflow(t, binary, harness, "focus", "show")
	if err != nil || !strings.Contains(output, "Task: "+second) {
		t.Fatalf("focus show = %q, err %v; want second task", output, err)
	}
	output, err = runJaflow(t, binary, harness, "focus", "pop")
	if err != nil || !strings.Contains(output, first) {
		t.Fatalf("focus pop = %q, err %v; want first task", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "focus", "clear"); err != nil {
		t.Fatalf("focus clear: %v\n%s", err, output)
	}
}

func TestSessionListShowsCurrentAnchor(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	planOutput, err := runJaflow(t, binary, harness, "plan", "sessions", "First")
	if err != nil {
		t.Fatalf("create session plan: %v\n%s", err, planOutput)
	}
	match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(planOutput)
	if len(match) != 2 {
		t.Fatalf("plan output = %q, want one task UUID", planOutput)
	}
	if output, err := runJaflow(t, binary, harness, "focus", "task", match[1]); err != nil {
		t.Fatalf("focus session task: %v\n%s", err, output)
	}
	output, err := runJaflow(t, binary, harness, "session", "list")
	if err != nil || !strings.Contains(output, "* session") || !strings.Contains(output, match[1]) {
		t.Fatalf("session list = %q, err %v; want current anchor", output, err)
	}
}

func TestDashboardShowsBlockedWorkAndBacklogLifecycle(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "dashboard", "First", "Second")
	if err != nil {
		t.Fatalf("create dashboard plan: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "ponder")
	if err != nil || !strings.Contains(output, "dashboard") || !strings.Contains(output, "blocked:1") {
		t.Fatalf("ponder = %q, err %v; want blocked dashboard count", output, err)
	}
	output, err = runJaflow(t, binary, harness, "tree", "dashboard")
	if err != nil || !strings.Contains(output, "READY") || !strings.Contains(output, "BLOCKED") {
		t.Fatalf("tree = %q, err %v; want dependency markers", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "backlog", "dashboard"); err != nil {
		t.Fatalf("backlog dashboard: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "plans")
	if err != nil || strings.Contains(output, "dashboard") {
		t.Fatalf("plans after backlog = %q, err %v; want hidden initiative", output, err)
	}
	output, err = runJaflow(t, binary, harness, "plans", "--with-backlog")
	if err != nil || !strings.Contains(output, "dashboard") {
		t.Fatalf("plans with backlog = %q, err %v; want initiative", output, err)
	}
}

func TestStatusUsesPromptAsAdCacheSignal(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	if output, err := runJaflow(t, binary, harness, "plan", "cache", "First"); err != nil {
		t.Fatalf("create cache plan: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "status"); err != nil {
		t.Fatalf("prime status cache: %v\n%s", err, output)
	}
	output, err := runJaflow(t, binary, harness, "status")
	if err != nil || !strings.Contains(output, "[cached]") {
		t.Fatalf("cached status = %q, err %v; want Prompt as Ad signal", output, err)
	}
}

func TestRoadmapInitializationHasDuplicateGuard(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	if output, err := runJaflow(t, binary, harness, "plan", "roadmap", "First"); err != nil {
		t.Fatalf("create roadmap initiative: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "roadmap", "init"); err != nil {
		t.Fatalf("initialize roadmap: %v\n%s", err, output)
	}
	output, err := runJaflow(t, binary, harness, "roadmap", "init")
	if err == nil || !strings.Contains(output, "already initialized") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("duplicate roadmap init = %q, err %v; want actionable guard", output, err)
	}
	output, err = runJaflow(t, binary, harness, "roadmap", "show")
	if err != nil || !strings.Contains(output, "ROADMAP") || !strings.Contains(output, "roadmap") {
		t.Fatalf("roadmap show = %q, err %v; want ledger entry", output, err)
	}
}

func TestTaskWritePreservesOtherInitiativeStatusCache(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	output, err := runJaflow(t, binary, harness, "plan", "cache-a", "Task A")
	if err != nil {
		t.Fatalf("create cache-a plan: %v\n%s", err, output)
	}
	match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("cache-a output = %q, want task UUID", output)
	}
	taskID := match[1]
	if output, err := runJaflow(t, binary, harness, "plan", "cache-b", "Task B"); err != nil {
		t.Fatalf("create cache-b plan: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-a"); err != nil {
		t.Fatalf("prime cache-a: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-b"); err != nil {
		t.Fatalf("prime cache-b: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "note", taskID, "decision", "Only A changed"); err != nil {
		t.Fatalf("annotate cache-a: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-b"); err != nil || !strings.Contains(output, "[cached]") {
		t.Fatalf("cache-b after cache-a write = %q, err %v; want cached", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-a"); err != nil || strings.Contains(output, "[cached]") {
		t.Fatalf("cache-a after own write = %q, err %v; want refresh", output, err)
	}
}

func TestRoadmapShipChangesPhase(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	if output, err := runJaflow(t, binary, harness, "plan", "roadmap", "Task"); err != nil {
		t.Fatalf("create roadmap plan: %v\n%s", err, output)
	}
	output, err := runJaflow(t, binary, harness, "roadmap", "init")
	if err != nil {
		t.Fatalf("initialize roadmap: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "roadmap", "add", "--phase", "next", "--description", "Ship this phase")
	if err != nil {
		t.Fatalf("add roadmap phase: %v\n%s", err, output)
	}
	match := regexp.MustCompile(`Roadmap phase added: \[next\] Ship this phase \(([^)]+)\)`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("roadmap add output = %q, want entry ID", output)
	}
	output, err = runJaflow(t, binary, harness, "roadmap", "ship", match[1])
	if err != nil || !strings.Contains(output, "Phase shipped: Ship this phase") {
		t.Fatalf("roadmap ship = %q, err %v; want shipped phase", output, err)
	}
	output, err = runJaflow(t, binary, harness, "roadmap", "show")
	if err != nil || !strings.Contains(output, "[shipped] Ship this phase") {
		t.Fatalf("roadmap after ship = %q, err %v; want shipped marker", output, err)
	}
}

func TestLifecycleAndFocusInvalidateStatusCache(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	output, err := runJaflow(t, binary, harness, "plan", "cache-lifecycle", "Task")
	if err != nil {
		t.Fatalf("create cache-lifecycle plan: %v\n%s", err, output)
	}
	match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("plan output = %q, want task UUID", output)
	}
	taskID := match[1]
	if output, err := runJaflow(t, binary, harness, "status", "cache-lifecycle"); err != nil {
		t.Fatalf("prime lifecycle cache: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "execute", taskID); err != nil {
		t.Fatalf("execute cache task: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-lifecycle"); err != nil || strings.Contains(output, "[cached]") {
		t.Fatalf("status after execute = %q, err %v; want refreshed cache", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-lifecycle"); err != nil || !strings.Contains(output, "[cached]") {
		t.Fatalf("reprimed lifecycle cache = %q, err %v; want cached status", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "outcome", taskID, "Ready"); err != nil {
		t.Fatalf("record cache outcome: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-lifecycle"); err != nil || strings.Contains(output, "[cached]") {
		t.Fatalf("status after outcome = %q, err %v; want refreshed cache", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-lifecycle"); err != nil {
		t.Fatalf("prime focus cache: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "focus", "task", taskID); err != nil {
		t.Fatalf("focus cache task: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-lifecycle"); err != nil || strings.Contains(output, "[cached]") {
		t.Fatalf("status after focus = %q, err %v; want refreshed cache", output, err)
	}
}

func TestActiveBlockedAndOverdueViews(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "views", "Active", "Blocked", "Overdue|testing|2000-01-01")
	if err != nil {
		t.Fatalf("create views plan: %v\n%s", err, output)
	}
	matches := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindAllStringSubmatch(output, -1)
	if len(matches) != 3 {
		t.Fatalf("views plan output = %q, want three task UUIDs", output)
	}
	if output, err := runJaflow(t, binary, harness, "execute", matches[0][1]); err != nil {
		t.Fatalf("execute active task: %v\n%s", err, output)
	}

	output, err = runJaflow(t, binary, harness, "active")
	if err != nil || !strings.Contains(output, "Active") || strings.Contains(output, "Blocked") {
		t.Fatalf("active view = %q, err %v; want only active task", output, err)
	}
	output, err = runJaflow(t, binary, harness, "blocked")
	if err != nil || !strings.Contains(output, "Blocked") || !strings.Contains(output, "Overdue") {
		t.Fatalf("blocked view = %q, err %v; want blocked tasks", output, err)
	}
	output, err = runJaflow(t, binary, harness, "overdue")
	if err != nil || !strings.Contains(output, "Overdue") || strings.Contains(output, "Blocked") {
		t.Fatalf("overdue view = %q, err %v; want only overdue task", output, err)
	}
}

func TestHandoffExecutesAndAnnotates(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "handoff", "First", "Second")
	if err != nil {
		t.Fatalf("create handoff plan: %v\n%s", err, output)
	}
	matches := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindAllStringSubmatch(output, -1)
	if len(matches) != 2 {
		t.Fatalf("handoff plan output = %q, want two task UUIDs", output)
	}
	first, second := matches[0][1], matches[1][1]
	for _, args := range [][]string{
		{"execute", first},
		{"outcome", first, "First complete"},
		{"done", first},
	} {
		if output, err := runJaflow(t, binary, harness, args...); err != nil {
			t.Fatalf("run %v: %v\n%s", args, err, output)
		}
	}
	if output, err := runJaflow(t, binary, harness, "handoff", second, "Start second"); err != nil {
		t.Fatalf("handoff second task: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "notes", second); err != nil || !strings.Contains(output, "HANDOFF: Start second") {
		t.Fatalf("handoff notes = %q, err %v; want HANDOFF annotation", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "tree", "handoff"); err != nil || !strings.Contains(output, "[ACTIVE] "+second) {
		t.Fatalf("handoff tree = %q, err %v; want active target", output, err)
	}
}

func TestIndependentFocusAndNativeSessionLifecycle(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "sessions", "Task")
	if err != nil {
		t.Fatalf("create session plan: %v\n%s", err, output)
	}
	match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("session plan output = %q, want task UUID", output)
	}
	taskID := match[1]
	if output, err := runJaflow(t, binary, harness, "focus", "ind", "task", taskID); err != nil {
		t.Fatalf("independent focus: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "session", "list"); err != nil || !strings.Contains(output, "* session") {
		t.Fatalf("session list = %q, err %v; want current session", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "session", "dump"); err != nil {
		t.Fatalf("session dump: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "session", "resume"); err != nil || !strings.Contains(output, "SESSION HANDOFF") {
		t.Fatalf("session resume = %q, err %v; want handoff", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "session", "dump"); err == nil || !strings.Contains(output, "unfilled") {
		t.Fatalf("duplicate session dump = %q, err %v; want unfilled guard", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "session", "ack"); err != nil {
		t.Fatalf("session ack: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "session", "resume"); err != nil || !strings.Contains(strings.ToLower(output), "already acknowledged") {
		t.Fatalf("acknowledged resume = %q, err %v; want acknowledged note", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "focus", "back"); err != nil || !strings.Contains(output, "Switched back to global focus") {
		t.Fatalf("focus back = %q, err %v; want global fallback", output, err)
	}
}

func TestExternalTicketInheritanceContracts(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "ticket", "Root", "Child")
	if err != nil {
		t.Fatalf("create ticket plan: %v\n%s", err, output)
	}
	matches := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindAllStringSubmatch(output, -1)
	if len(matches) != 2 {
		t.Fatalf("ticket plan output = %q, want two task UUIDs", output)
	}
	root, child := matches[0][1], matches[1][1]

	if output, err := runJaflow(t, binary, harness, "ticket", root, "#PARENT-123"); err != nil {
		t.Fatalf("set parent ticket: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "focus", "task", child); err != nil {
		t.Fatalf("focus child task: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "status", "ticket")
	for _, expected := range []string{
		"[#PARENT-123] Root",
		"[#PARENT-123] Child",
		"Inherited ticket detected (#PARENT-123)",
	} {
		if err != nil || !strings.Contains(output, expected) {
			t.Fatalf("inherited ticket status = %q, err %v; want %q", output, err, expected)
		}
	}
	if output, err := runJaflow(t, binary, harness, "commit"); err != nil || !strings.Contains(output, "Refs: #PARENT-123") {
		t.Fatalf("inherited ticket commit = %q, err %v; want parent reference", output, err)
	}

	if output, err := runJaflow(t, binary, harness, "ticket", child, "#CHILD-456"); err != nil {
		t.Fatalf("set child ticket: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "status", "ticket", "--force")
	if err != nil || !strings.Contains(output, "[#CHILD-456] Child") {
		t.Fatalf("direct ticket status = %q, err %v; want child ticket", output, err)
	}
}

func TestNoteAndContextContracts(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "context", "Root", "Middle", "Leaf")
	if err != nil {
		t.Fatalf("create context plan: %v\n%s", err, output)
	}
	matches := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindAllStringSubmatch(output, -1)
	if len(matches) != 3 {
		t.Fatalf("context plan output = %q, want three task UUIDs", output)
	}
	root, middle, leaf := matches[0][1], matches[1][1], matches[2][1]

	if output, err := runJaflow(t, binary, harness, "note", root, "decision", "Root decision"); err != nil {
		t.Fatalf("add decision note: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "note", middle, "research", "Middle research"); err != nil {
		t.Fatalf("add research note: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "note", root, "question", "Why root?"); err != nil {
		t.Fatalf("add question note: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "note", root, "hypothesis", "Root may explain it"); err != nil {
		t.Fatalf("add hypothesis note: %v\n%s", err, output)
	}

	notes, err := runJaflow(t, binary, harness, "notes", root)
	if err != nil || !strings.Contains(notes, "DECISION: Root decision") {
		t.Fatalf("notes output = %q, err %v; want decision annotation", notes, err)
	}
	timestampMatch := regexp.MustCompile(`\[([^]]+)\] DECISION: Root decision`).FindStringSubmatch(notes)
	if len(timestampMatch) != 2 {
		t.Fatalf("notes output = %q, want timestamped decision", notes)
	}

	contextOutput, err := runJaflow(t, binary, harness, "context", leaf)
	if err != nil || !strings.Contains(contextOutput, "Root") {
		t.Fatalf("context output = %q, err %v; want target context", contextOutput, err)
	}
	if output, err := runJaflow(t, binary, harness, "focus", "task", leaf); err != nil {
		t.Fatalf("focus leaf task: %v\n%s", err, output)
	}
	statusOutput, err := runJaflow(t, binary, harness, "status", "context")
	for _, expected := range []string{
		"INHERITED CONTEXT",
		"DECISION: Root decision",
		"RESEARCH: Middle research",
		"QUESTION: Why root?",
		"HYPOTHESIS: Root may explain it",
	} {
		if err != nil || !strings.Contains(statusOutput, expected) {
			t.Fatalf("status output = %q, err %v; want %q", statusOutput, err, expected)
		}
	}

	if output, err := runJaflow(t, binary, harness, "note", root, "delete", timestampMatch[1]); err != nil {
		t.Fatalf("delete annotation: %v\n%s", err, output)
	}
	remaining, err := runJaflow(t, binary, harness, "notes", root)
	if err != nil || strings.Contains(remaining, "Root decision") {
		t.Fatalf("remaining notes = %q, err %v; deleted annotation remains", remaining, err)
	}

	if output, err := runJaflow(t, binary, harness, "note", leaf, "note", "Direct context"); err != nil {
		t.Fatalf("add direct note: %v\n%s", err, output)
	}
	contextOutput, err = runJaflow(t, binary, harness, "context", leaf)
	if err != nil || !strings.Contains(contextOutput, "NOTE: Direct context") {
		t.Fatalf("direct context output = %q, err %v; want direct note", contextOutput, err)
	}
}

func TestNotesAllowCompletedTasksAndRejectInvalidKinds(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "completed-notes", "Task")
	if err != nil {
		t.Fatalf("create completed-notes plan: %v\n%s", err, output)
	}
	match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("plan output = %q, want task UUID", output)
	}
	taskID := match[1]

	if output, err := runJaflow(t, binary, harness, "note", taskID, "invalid", "Message"); err == nil || !strings.Contains(output, "ACTION: Use one of the allowed semantic types") {
		t.Fatalf("invalid note output = %q, err %v; want actionable error", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "execute", taskID); err != nil {
		t.Fatalf("execute task: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "outcome", taskID, "Completed"); err != nil {
		t.Fatalf("record outcome: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "done", taskID); err != nil {
		t.Fatalf("complete task: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "note", taskID, "note", "Post-completion note"); err != nil {
		t.Fatalf("add post-completion note: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "notes", taskID); err != nil || !strings.Contains(output, "NOTE: Post-completion note") {
		t.Fatalf("completed task notes = %q, err %v; want note", output, err)
	}
	if output, err := runJaflow(t, binary, harness, "ticket", taskID, "#LATE-1"); err == nil || !strings.Contains(output, "already COMPLETED") {
		t.Fatalf("completed task ticket = %q, err %v; want lifecycle protection", output, err)
	}
}

func TestNoteInvalidatesFocusedStatusCache(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	output, err := runJaflow(t, binary, harness, "plan", "cache-notes", "Task")
	if err != nil {
		t.Fatalf("create cache-notes plan: %v\n%s", err, output)
	}
	match := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("plan output = %q, want task UUID", output)
	}
	taskID := match[1]
	if output, err := runJaflow(t, binary, harness, "focus", "task", taskID); err != nil {
		t.Fatalf("focus cache task: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "status", "cache-notes"); err != nil {
		t.Fatalf("prime status cache: %v\n%s", err, output)
	}
	if output, err := runJaflow(t, binary, harness, "note", taskID, "decision", "Cache changed"); err != nil {
		t.Fatalf("add cache note: %v\n%s", err, output)
	}
	output, err = runJaflow(t, binary, harness, "status", "cache-notes")
	if err != nil || strings.Contains(output, "[cached]") || !strings.Contains(output, "DECISION: Cache changed") {
		t.Fatalf("status after note = %q, err %v; want refreshed context", output, err)
	}
}
