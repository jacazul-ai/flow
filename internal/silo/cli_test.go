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

func runSilo(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := silo.Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunGeneratesUniformChainsIntoANewDatabase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "silo")
	database := filepath.Join(dir, "flow.sqlite3")

	code, stdout, stderr := runSilo("--database", database, "--project", "bench",
		"--chains", "2", "--tasks", "3", "--completed", "1", "--active", "--ticket", "2=#MID-2")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	for _, want := range []string{"Generated silo " + database, "project: bench | chains: 2 | tasks per chain: 3", "JACAZUL_PROJECT=bench JACAZUL_FLOW_DATABASE_PATH=" + database} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}

	env := flow.Env{ProjectID: "bench", SessionID: "check", DatabasePath: database, Home: dir}
	plans := report(t, env, "plans", "--force")
	for _, want := range []string{"chain-1 [id:", "chain-2 [id:", "pending:1 active:1 completed:1 blocked:1"} {
		if !strings.Contains(plans, want) {
			t.Fatalf("plans = %q, want %q", plans, want)
		}
	}
	if status := report(t, env, "status", "chain-1", "--force"); !strings.Contains(status, "[#MID-2] chain-1 task 2") {
		t.Fatalf("status = %q, want the ticket at position 2", status)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read silo directory: %v", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "flow.sqlite3") {
			t.Fatalf("silo directory holds %q; generation must write only the database", entry.Name())
		}
	}
}

func TestRunRefusesAnExistingDatabase(t *testing.T) {
	database := filepath.Join(t.TempDir(), "flow.sqlite3")
	if err := os.WriteFile(database, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	code, stdout, stderr := runSilo("--database", database)
	if code != 1 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q; want 1 and no output", code, stdout)
	}
	if !strings.Contains(stderr, "already exists") || !strings.Contains(stderr, "ACTION:") {
		t.Fatalf("stderr = %q, want an actionable refusal", stderr)
	}
	if content, err := os.ReadFile(database); err != nil || string(content) != "keep me" {
		t.Fatalf("existing file = %q, err %v; want it untouched", content, err)
	}
}

func TestRunRequiresAnExplicitDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("JACAZUL_HOME", home)

	code, _, stderr := runSilo("--chains", "1")
	if code != 1 || !strings.Contains(stderr, "--database is required") || !strings.Contains(stderr, "ACTION:") {
		t.Fatalf("exit = %d, stderr = %q; want an actionable missing --database error", code, stderr)
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("home holds %d entries; a refused run must not write anything", len(entries))
	}
}

func TestRunRejectsInvalidInputWithoutLeavingADatabase(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{name: "completed beyond size", args: []string{"--tasks", "2", "--completed", "3"}, code: 1, want: "completes 3 of 2"},
		{name: "no chains", args: []string{"--chains", "0"}, code: 1, want: "must be at least 1"},
		{name: "ticket without position", args: []string{"--ticket", "#X"}, code: 2, want: "POSITION=TICKET"},
		{name: "ticket position zero", args: []string{"--ticket", "0=#X"}, code: 2, want: "position from 1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := filepath.Join(t.TempDir(), "flow.sqlite3")

			code, _, stderr := runSilo(append([]string{"--database", database}, test.args...)...)
			if code != test.code || !strings.Contains(stderr, test.want) || !strings.Contains(stderr, "ACTION:") {
				t.Fatalf("exit = %d, stderr = %q; want %d with %q and an ACTION", code, stderr, test.code, test.want)
			}
			if _, err := os.Stat(database); !os.IsNotExist(err) {
				t.Fatalf("database exists after rejected input (stat: %v)", err)
			}
		})
	}
}
