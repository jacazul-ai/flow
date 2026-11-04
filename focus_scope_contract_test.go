package flow_test

import (
	"database/sql"
	"errors"
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

// sessionScope reads the tracked scope of one session of the harness project.
func sessionScope(t *testing.T, harness *testharness.Harness, sessionID string) string {
	t.Helper()
	db, err := sql.Open("sqlite", harness.DatabasePath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	var scope string
	err = db.QueryRow("SELECT scope FROM sessions WHERE project_id = ? AND session_id = ?", harness.ProjectID, sessionID).Scan(&scope)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatalf("read session scope: %v", err)
	}
	return scope
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
	if got := sessionScope(t, fx.first, "first"); got != "global" {
		t.Fatalf("first session scope = %q, want none: a plain focus must stay global", got)
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
	if got := sessionScope(t, fx.first, "first"); got != "global" {
		t.Fatalf("first session scope = %q, want none after plain focus operations", got)
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

func TestGlobalSessionKeepsItsOwnHandoff(t *testing.T) {
	fx := newScopeFixture(t)
	mustFlow(t, fx.first, "focus", "task", fx.tasks[0])

	output := mustFlow(t, fx.first, "session", "dump")
	dumpID := regexp.MustCompile(`Session dump draft ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(dumpID) != 2 {
		t.Fatalf("session dump = %q, want draft UUID", output)
	}
	if !strings.Contains(output, "**Session:** first\n**Scope:** global") || !strings.Contains(output, fx.tasks[0]) {
		t.Fatalf("session dump = %q, want the session's own handoff of the global focus", output)
	}
	if output, err := runFlow(t, fx.second, "session", "finalize", dumpID[1], "--actions", "x"); err == nil {
		t.Fatalf("second session finalize = %q, want the first session's draft out of reach", output)
	}
	mustFlow(t, fx.first, "session", "finalize", dumpID[1], "--actions", "Continue the first session")

	if got := sessionScope(t, fx.first, "first"); got != "global" {
		t.Fatalf("first session scope = %q, want the dump to keep it in the global focus", got)
	}
	mustFlow(t, fx.second, "focus", "task", fx.tasks[1])
	if got := focusedTask(t, fx.first); got != fx.tasks[1] {
		t.Fatalf("first session focus = %s after a dump, want the shared %s", got, fx.tasks[1])
	}
	for _, harness := range []*testharness.Harness{fx.second, fx.foreign} {
		if output := mustFlow(t, harness, "session", "resume"); strings.Contains(output, "Continue the first session") {
			t.Fatalf("%s resume = %q, want no handoff of another session", harness.SessionID, output)
		}
		if output := mustFlow(t, harness, "session", "dumps"); strings.Contains(output, dumpID[1]) {
			t.Fatalf("%s dumps = %q, want only its own", harness.SessionID, output)
		}
	}
	if output := mustFlow(t, fx.first, "session", "resume"); !strings.Contains(output, "Continue the first session") {
		t.Fatalf("first session resume = %q, want its own handoff", output)
	}
}

func dumpAndFinalize(t *testing.T, harness *testharness.Harness, actions string) string {
	t.Helper()
	output := mustFlow(t, harness, "session", "dump")
	dumpID := regexp.MustCompile(`Session dump draft ([0-9a-f]{8})`).FindStringSubmatch(output)
	if len(dumpID) != 2 {
		t.Fatalf("session dump = %q, want draft UUID", output)
	}
	mustFlow(t, harness, "session", "finalize", dumpID[1], "--actions", actions)
	return dumpID[1]
}

func TestHandoffIsKeyedBySessionAndScope(t *testing.T) {
	fx := newScopeFixture(t)
	mustFlow(t, fx.first, "focus", "task", fx.tasks[0])
	globalDump := dumpAndFinalize(t, fx.first, "Global handoff of first")

	mustFlow(t, fx.first, "focus", "ind", "task", fx.tasks[1])
	if output := mustFlow(t, fx.first, "session", "resume"); strings.Contains(output, "Global handoff of first") {
		t.Fatalf("independent resume = %q, want the global handoff out of the independent scope", output)
	}
	indDump := dumpAndFinalize(t, fx.first, "Independent handoff of first")
	if output := mustFlow(t, fx.first, "session", "resume"); !strings.Contains(output, "Independent handoff of first") || !strings.Contains(output, "**Scope:** ind") {
		t.Fatalf("independent resume = %q, want the independent handoff", output)
	}
	if output := mustFlow(t, fx.first, "session", "dumps"); !strings.Contains(output, indDump) || strings.Contains(output, globalDump) {
		t.Fatalf("independent dumps = %q, want only the independent scope", output)
	}

	mustFlow(t, fx.first, "focus", "back")
	if output := mustFlow(t, fx.first, "session", "resume"); !strings.Contains(output, "Global handoff of first") {
		t.Fatalf("resume after focus back = %q, want the session's global handoff", output)
	}
	mustFlow(t, fx.first, "session", "ack")
	mustFlow(t, fx.first, "focus", "ind", "task", fx.tasks[1])
	if output := mustFlow(t, fx.first, "session", "resume"); !strings.Contains(output, "Independent handoff of first") {
		t.Fatalf("resume after focus ind = %q, want the independent handoff still pending", output)
	}

	for _, harness := range []*testharness.Harness{fx.second, fx.foreign} {
		output := mustFlow(t, harness, "session", "resume")
		if strings.Contains(output, "handoff of first") {
			t.Fatalf("%s resume = %q, want no handoff of another session", harness.SessionID, output)
		}
	}
	all := mustFlow(t, fx.first, "session", "dumps", "--all")
	if !regexp.MustCompile(`first global acknowledged `+globalDump).MatchString(all) ||
		!regexp.MustCompile(`first ind pending `+indDump).MatchString(all) {
		t.Fatalf("session dumps --all = %q, want the scope of each dump", all)
	}
	if output := mustFlow(t, fx.foreign, "session", "dumps", "--all"); strings.Contains(output, globalDump) || strings.Contains(output, indDump) {
		t.Fatalf("foreign dumps --all = %q, want project isolation", output)
	}
}

func TestPendingHandoffGuardIsPerScope(t *testing.T) {
	fx := newScopeFixture(t)
	mustFlow(t, fx.first, "focus", "task", fx.tasks[0])
	dumpAndFinalize(t, fx.first, "Global pending")

	if output, err := runFlow(t, fx.first, "session", "dump"); err == nil || !strings.Contains(output, "ACTION:") {
		t.Fatalf("second global dump = %q, err %v; want the pending guard", output, err)
	}
	mustFlow(t, fx.second, "session", "dump")
	mustFlow(t, fx.first, "focus", "ind", "task", fx.tasks[1])
	mustFlow(t, fx.first, "session", "dump")
}

func sessionListRow(t *testing.T, output string, sessionID string) []string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimPrefix(line, "* "))
		if len(fields) > 0 && fields[0] == sessionID {
			return fields
		}
	}
	return nil
}

func TestSessionListTracksEverySessionScope(t *testing.T) {
	fx := newScopeFixture(t)
	reader := *fx.first
	reader.SessionID = "reader"
	mustFlow(t, fx.first, "focus", "task", fx.tasks[0])
	mustFlow(t, fx.second, "focus", "ind", "task", fx.tasks[1])
	mustFlow(t, &reader, "status")
	mustFlow(t, &reader, "focus")

	output := mustFlow(t, fx.first, "session", "list")
	if row := sessionListRow(t, output, "first"); len(row) < 4 || row[1] != "global" || row[2] != "scope" || row[3] != fx.tasks[0] {
		t.Fatalf("session list = %q, want first in the global focus on %s", output, fx.tasks[0])
	}
	if row := sessionListRow(t, output, "second"); len(row) < 4 || row[1] != "ind" || row[3] != fx.tasks[1] {
		t.Fatalf("session list = %q, want second in its own focus on %s", output, fx.tasks[1])
	}
	if row := sessionListRow(t, output, "reader"); row != nil {
		t.Fatalf("session list = %q, want reads to register nothing", output)
	}
	if row := sessionListRow(t, output, "foreign-session"); row != nil {
		t.Fatalf("session list = %q, want project isolation", output)
	}
	if output := mustFlow(t, fx.foreign, "session", "list"); sessionListRow(t, output, "first") != nil || sessionListRow(t, output, "second") != nil {
		t.Fatalf("foreign session list = %q, want project isolation", output)
	}

	mustFlow(t, fx.second, "focus", "back")
	output = mustFlow(t, fx.first, "session", "list")
	if row := sessionListRow(t, output, "second"); len(row) < 4 || row[1] != "global" || row[3] != fx.tasks[0] {
		t.Fatalf("session list after focus back = %q, want second back in the global focus", output)
	}
	if output := mustFlow(t, fx.first, "session", "list", "--format", "json"); !strings.Contains(output, `"scope":"global"`) || !strings.Contains(output, `"state":"active"`) {
		t.Fatalf("structured session list = %q, want scope and state", output)
	}
}

func TestPurgedSessionNeverActsAgainButStaysVisible(t *testing.T) {
	fx := newScopeFixture(t)
	mustFlow(t, fx.second, "focus", "task", fx.tasks[0])
	db, err := sql.Open("sqlite", fx.first.DatabasePath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if _, err := db.Exec("UPDATE sessions SET updated_at = '2020-01-01T00:00:00Z' WHERE session_id = 'second'"); err != nil {
		t.Fatalf("age second session: %v", err)
	}
	db.Close()
	if output := mustFlow(t, fx.first, "session", "purge"); !strings.Contains(output, "Marked 1 orphan session(s) purged") {
		t.Fatalf("session purge = %q, want the orphan purged", output)
	}

	for _, args := range [][]string{
		{"focus", "task", fx.tasks[1]},
		{"focus", "ind", "task", fx.tasks[1]},
		{"focus", "back"},
		{"session", "dump"},
		{"session", "resume"},
		{"onboard"},
	} {
		output, err := runFlow(t, fx.second, args...)
		if err == nil || !strings.Contains(output, "is purged") || !strings.Contains(output, "ACTION:") {
			t.Fatalf("purged %s = %q, err %v; want a refusal", strings.Join(args, " "), output, err)
		}
	}
	if got := focusedTask(t, fx.first); got != fx.tasks[0] {
		t.Fatalf("global focus = %s, want the purged session's write refused", got)
	}
	if output := mustFlow(t, fx.first, "session", "list"); sessionListRow(t, output, "second") != nil {
		t.Fatalf("session list = %q, want the purged session hidden", output)
	}
	output := mustFlow(t, fx.first, "session", "list", "--all")
	if row := sessionListRow(t, output, "second"); len(row) < 6 || row[1] != "global" || row[5] != "purged" {
		t.Fatalf("session list --all = %q, want the purged session visible", output)
	}
}
