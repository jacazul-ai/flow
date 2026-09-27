package silo_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	flow "github.com/jacazul-ai/flow"
	"github.com/jacazul-ai/flow/internal/silo"
)

func report(t *testing.T, env flow.Env, args ...string) string {
	t.Helper()

	var stdout, stderr bytes.Buffer
	if code := flow.Run(context.Background(), args, env, flow.Streams{Stdout: &stdout, Stderr: &stderr}); code != 0 {
		t.Fatalf("run %v: exit %d: %s", args, code, stderr.String())
	}
	return stdout.String()
}

func TestGenerateBuildsRequestedStates(t *testing.T) {
	env := flow.Env{ProjectID: "project", SessionID: "session", Home: t.TempDir()}

	ids, err := silo.Generate(context.Background(), env, []silo.Chain{{
		Name:      "chain",
		Tasks:     []string{"T1", "T2", "T3", "T4"},
		Tickets:   map[int]string{1: "#MID-1"},
		Completed: 1,
		Active:    true,
	}})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(ids) != 4 {
		t.Fatalf("ids = %v, want four tasks", ids)
	}

	want := "══ Plan: chain ══\nPENDING:\n" +
		"- [" + ids["T2"] + "] [#MID-1] T2\n" +
		"- [" + ids["T3"] + "] [#MID-1] T3\n" +
		"- [" + ids["T4"] + "] [#MID-1] T4\n" +
		"COMPLETED:\n" +
		"- [" + ids["T1"] + "] T1\n"
	if got := report(t, env, "status", "chain", "--force"); got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got := report(t, env, "plans", "--force"); !strings.Contains(got, "chain [id:") ||
		!strings.Contains(got, "pending:2 active:1 completed:1 blocked:2") {
		t.Fatalf("plans = %q, want one active, one completed and two blocked tasks", got)
	}
}

func TestGenerateStaysInsideItsProject(t *testing.T) {
	home := t.TempDir()
	alpha := flow.Env{ProjectID: "alpha", SessionID: "session", Home: home}
	beta := flow.Env{ProjectID: "beta", SessionID: "session", Home: home}

	if _, err := silo.Generate(context.Background(), alpha, []silo.Chain{{Name: "alpha-chain", Tasks: []string{"A1"}}}); err != nil {
		t.Fatalf("generate alpha: %v", err)
	}
	if got := report(t, beta, "plans", "--force"); strings.Contains(got, "alpha-chain") {
		t.Fatalf("beta plans = %q, saw alpha's silo", got)
	}

	if _, err := silo.Generate(context.Background(), beta, []silo.Chain{{Name: "beta-chain", Tasks: []string{"B1"}}}); err != nil {
		t.Fatalf("generate beta: %v", err)
	}
	if got := report(t, alpha, "plans", "--force"); strings.Contains(got, "beta-chain") || !strings.Contains(got, "alpha-chain") {
		t.Fatalf("alpha plans = %q, want only alpha's silo", got)
	}
}

func TestGenerateRejectsInvalidChainsBeforeWriting(t *testing.T) {
	tests := []struct {
		name   string
		chains []silo.Chain
		want   string
	}{
		{name: "missing name", chains: []silo.Chain{{Tasks: []string{"T1"}}}, want: "needs a name"},
		{name: "no tasks", chains: []silo.Chain{{Name: "c"}}, want: "at least one task"},
		{name: "too many completed", chains: []silo.Chain{{Name: "c", Tasks: []string{"T1"}, Completed: 2}}, want: "completes 2 of 1"},
		{name: "negative completed", chains: []silo.Chain{{Name: "c", Tasks: []string{"T1"}, Completed: -1}}, want: "completes -1 of 1"},
		{name: "nothing to start", chains: []silo.Chain{{Name: "c", Tasks: []string{"T1"}, Completed: 1, Active: true}}, want: "no task left to start"},
		{name: "ticket out of range", chains: []silo.Chain{{Name: "c", Tasks: []string{"T1"}, Tickets: map[int]string{1: "#X"}}}, want: "index 1 outside"},
		{
			name:   "duplicate description",
			chains: []silo.Chain{{Name: "c", Tasks: []string{"T1"}}, {Name: "d", Tasks: []string{"T1"}}},
			want:   "must be unique",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := filepath.Join(t.TempDir(), "flow.sqlite3")
			env := flow.Env{ProjectID: "project", SessionID: "session", DatabasePath: database, Home: t.TempDir()}

			_, err := silo.Generate(context.Background(), env, test.chains)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if _, statErr := os.Stat(database); !os.IsNotExist(statErr) {
				t.Fatalf("database exists after a rejected spec (stat: %v)", statErr)
			}
		})
	}
}
