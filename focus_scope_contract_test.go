package flow_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/testharness"
)

// scopeFixture holds two sessions of one project and one session of another
// project, all sharing one database so the SQL project boundary is exercised.
type scopeFixture struct {
	first   *testharness.Harness
	second  *testharness.Harness
	foreign *testharness.Harness
	tasks   []string
}

func newScopeFixture(t *testing.T) scopeFixture {
	t.Helper()
	first := testharness.NewHarness(t, "project", "first")
	second := *first
	second.SessionID = "second"
	foreign := *first
	foreign.ProjectID = "foreign"
	foreign.SessionID = "foreign-session"

	output, err := runFlow(t, first, "plan", "scope", "First", "Second")
	if err != nil {
		t.Fatalf("create plan: %v\n%s", err, output)
	}
	matches := regexp.MustCompile(`Created task ([0-9a-f]{8})`).FindAllStringSubmatch(output, -1)
	if len(matches) != 2 {
		t.Fatalf("plan output = %q, want two task UUIDs", output)
	}
	if output, err := runFlow(t, &foreign, "plan", "foreign-scope", "Foreign"); err != nil {
		t.Fatalf("create foreign plan: %v\n%s", err, output)
	}
	return scopeFixture{
		first:   first,
		second:  &second,
		foreign: &foreign,
		tasks:   []string{matches[0][1], matches[1][1]},
	}
}

func mustFlow(t *testing.T, harness *testharness.Harness, args ...string) string {
	t.Helper()
	output, err := runFlow(t, harness, args...)
	if err != nil {
		t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}

func focusedTask(t *testing.T, harness *testharness.Harness) string {
	t.Helper()
	output := mustFlow(t, harness, "focus")
	match := regexp.MustCompile(`(?m)^Task: (\S+)$`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("focus output = %q, want a Task line", output)
	}
	return match[1]
}

func TestPlainFocusWritesTheSharedGlobalScope(t *testing.T) {
	fx := newScopeFixture(t)

	mustFlow(t, fx.first, "focus", "task", fx.tasks[1])

	if got := focusedTask(t, fx.second); got != fx.tasks[1] {
		t.Fatalf("second session focus = %s, want %s from the global focus", got, fx.tasks[1])
	}
	if got := countSessionRows(t, fx.first.DatabasePath, "sessions", "first"); got != 0 {
		t.Fatalf("first session rows = %d, want none: a plain focus must stay global", got)
	}
	if got := focusedTask(t, fx.foreign); got != "none" {
		t.Fatalf("foreign project focus = %s, want none", got)
	}

	mustFlow(t, fx.foreign, "focus", "plan", "foreign-scope")
	if got := focusedTask(t, fx.first); got != fx.tasks[1] {
		t.Fatalf("first session focus = %s after a foreign focus, want %s", got, fx.tasks[1])
	}
}

func TestEveryPlainFocusOperationKeepsTheGlobalScope(t *testing.T) {
	fx := newScopeFixture(t)

	mustFlow(t, fx.first, "focus", "scope")
	mustFlow(t, fx.first, "focus", "task", fx.tasks[1])
	mustFlow(t, fx.first, "focus", "pop")
	if got := focusedTask(t, fx.second); got != fx.tasks[0] {
		t.Fatalf("second session focus after pop = %s, want %s", got, fx.tasks[0])
	}
	mustFlow(t, fx.first, "focus", "interest", "add", "scope")
	if output := mustFlow(t, fx.second, "focus", "interest", "list"); !strings.Contains(output, "scope") {
		t.Fatalf("second session interests = %q, want the shared interest", output)
	}
	mustFlow(t, fx.first, "focus", "clear")
	if got := focusedTask(t, fx.second); got != "none" {
		t.Fatalf("second session focus after clear = %s, want none", got)
	}
	if got := countSessionRows(t, fx.first.DatabasePath, "sessions", "first"); got != 0 {
		t.Fatalf("first session rows = %d, want none after plain focus operations", got)
	}
}

func TestIndependentFocusStaysIndependentUntilFocusBack(t *testing.T) {
	fx := newScopeFixture(t)
	mustFlow(t, fx.second, "focus", "task", fx.tasks[0])

	mustFlow(t, fx.first, "focus", "ind", "task", fx.tasks[0])
	mustFlow(t, fx.first, "focus", "task", fx.tasks[1])

	if got := focusedTask(t, fx.first); got != fx.tasks[1] {
		t.Fatalf("independent focus = %s, want %s", got, fx.tasks[1])
	}
	if got := focusedTask(t, fx.second); got != fx.tasks[0] {
		t.Fatalf("global focus = %s, want %s untouched by the independent session", got, fx.tasks[0])
	}

	if output := mustFlow(t, fx.first, "focus", "back"); !strings.Contains(output, "Switched back to global focus") {
		t.Fatalf("focus back = %q, want global fallback", output)
	}
	mustFlow(t, fx.first, "focus", "task", fx.tasks[1])
	if got := focusedTask(t, fx.second); got != fx.tasks[1] {
		t.Fatalf("global focus = %s after focus back, want %s", got, fx.tasks[1])
	}
}

func TestFocusBackRefusesASessionAlreadyInTheGlobalFocus(t *testing.T) {
	fx := newScopeFixture(t)
	mustFlow(t, fx.first, "focus", "task", fx.tasks[0])

	output, err := runFlow(t, fx.first, "focus", "back")
	if err == nil || strings.Contains(output, "Switched back") {
		t.Fatalf("focus back = %q, err %v; want a refusal", output, err)
	}
	if !strings.Contains(output, "already in the global focus") || !strings.Contains(output, "ACTION:") {
		t.Fatalf("focus back = %q, want guidance toward focus ind", output)
	}
	if got := focusedTask(t, fx.second); got != fx.tasks[0] {
		t.Fatalf("global focus = %s after a refused focus back, want %s", got, fx.tasks[0])
	}
}

func TestGlobalFocusChangeInvalidatesEveryGlobalSessionCache(t *testing.T) {
	fx := newScopeFixture(t)
	independent := *fx.first
	independent.SessionID = "independent"
	mustFlow(t, fx.first, "focus", "task", fx.tasks[0])
	mustFlow(t, &independent, "focus", "ind", "task", fx.tasks[0])
	mustFlow(t, fx.foreign, "focus", "plan", "foreign-scope")
	for _, harness := range []*testharness.Harness{fx.second, &independent, fx.foreign} {
		mustFlow(t, harness, "status")
		if output := mustFlow(t, harness, "status"); !strings.Contains(output, "[cached]") {
			t.Fatalf("%s status = %q, want a warm cache", harness.SessionID, output)
		}
	}

	mustFlow(t, fx.first, "focus", "task", fx.tasks[1])

	if output := mustFlow(t, fx.second, "status"); strings.Contains(output, "[cached]") {
		t.Fatalf("global session status = %q, want a refresh after the global focus changed", output)
	}
	for _, harness := range []*testharness.Harness{&independent, fx.foreign} {
		if output := mustFlow(t, harness, "status"); !strings.Contains(output, "[cached]") {
			t.Fatalf("%s status = %q, want its cache kept", harness.SessionID, output)
		}
	}
}

func TestSessionHandoffFollowsTheActiveScope(t *testing.T) {
	fx := newScopeFixture(t)
	independent := *fx.first
	independent.SessionID = "independent"
	mustFlow(t, &independent, "focus", "ind", "task", fx.tasks[1])
	mustFlow(t, fx.first, "focus", "task", fx.tasks[0])

	output := mustFlow(t, fx.first, "session", "dump")
	dumpID := regexp.MustCompile(`Session dump draft ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(dumpID) != 2 {
		t.Fatalf("session dump = %q, want draft UUID", output)
	}
	mustFlow(t, fx.second, "session", "finalize", dumpID[1], "--actions", "Continue the global work")
	if got := countSessionRows(t, fx.first.DatabasePath, "sessions", "first"); got != 0 {
		t.Fatalf("first session rows = %d, want the dump to keep it global", got)
	}

	if output := mustFlow(t, fx.second, "session", "resume"); !strings.Contains(output, "Continue the global work") {
		t.Fatalf("second session resume = %q, want the global handoff", output)
	}
	for _, harness := range []*testharness.Harness{&independent, fx.foreign} {
		if output := mustFlow(t, harness, "session", "resume"); strings.Contains(output, "Continue the global work") {
			t.Fatalf("%s resume = %q, want no global handoff", harness.SessionID, output)
		}
	}
	mustFlow(t, fx.second, "session", "ack")
	if output := mustFlow(t, fx.first, "session", "resume"); !strings.Contains(output, "already acknowledged") {
		t.Fatalf("first session resume = %q, want the shared acknowledgement", output)
	}
	if output := mustFlow(t, fx.first, "session", "dumps"); !strings.Contains(output, dumpID[1]) {
		t.Fatalf("first session dumps = %q, want the global dump", output)
	}
	if output := mustFlow(t, &independent, "session", "dumps"); strings.Contains(output, dumpID[1]) {
		t.Fatalf("independent session dumps = %q, want only its own", output)
	}
}
