package testharness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewHarnessProvidesDistinctFixtureRoots(t *testing.T) {
	first := NewHarness(t, "project-alpha", "session-one")
	second := NewHarness(t, "project-beta", "session-two")

	firstFile := first.WriteFile(t, "state/focus.json", []byte("alpha"))
	secondFile := second.WriteFile(t, "state/focus.json", []byte("beta"))

	if first.Root == second.Root {
		t.Fatal("harnesses must use different roots")
	}
	if first.TaskData == second.TaskData {
		t.Fatal("harnesses must use different task data directories")
	}

	firstData, err := os.ReadFile(firstFile)
	if err != nil {
		t.Fatalf("read first fixture state: %v", err)
	}
	if string(firstData) != "alpha" {
		t.Fatalf("first fixture state = %q, want %q", firstData, "alpha")
	}

	secondData, err := os.ReadFile(secondFile)
	if err != nil {
		t.Fatalf("read second fixture state: %v", err)
	}
	if string(secondData) != "beta" {
		t.Fatalf("second fixture state = %q, want %q", secondData, "beta")
	}

	if _, err := os.Stat(filepath.Join(first.Root, "state", "focus.json")); err != nil {
		t.Fatalf("first fixture state is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(second.Root, "state", "focus.json")); err != nil {
		t.Fatalf("second fixture state is missing: %v", err)
	}
}

func TestFakeCommandUsesIsolatedPath(t *testing.T) {
	harness := NewHarness(t, "project", "session")
	harness.FakeCommand(t, "fake-task", "#!/bin/sh\nprintf 'task:%s\\n' \"$1\"\n")

	output, err := harness.RunCommand(context.Background(), "fake-task", "export")
	if err != nil {
		t.Fatalf("run fake task: %v", err)
	}
	if output != "task:export\n" {
		t.Fatalf("fake task output = %q, want %q", output, "task:export\\n")
	}
}

func TestFakeCommandsResolveOnlyThroughIsolatedPath(t *testing.T) {
	harness := NewHarness(t, "project-alpha", "session")
	harness.FakeCommand(t, "fake-inner", "#!/bin/sh\nprintf 'inner\\n'\n")
	harness.FakeCommand(t, "fake-outer", "#!/bin/sh\nfake-inner\n")

	output, err := harness.RunCommand(context.Background(), "fake-outer")
	if err != nil {
		t.Fatalf("run fake outer command: %v", err)
	}
	if output != "inner\n" {
		t.Fatalf("nested fake output = %q, want inner output", output)
	}
}

func TestHarnessCommandEnvironmentUsesFixtureValues(t *testing.T) {
	harness := NewHarness(t, "project-alpha", "session")
	harness.FakeCommand(t, "print-project", "#!/bin/sh\nprintf '%s\\n' \"$PROJECT_ID\"\n")

	output, err := harness.RunCommand(context.Background(), "print-project")
	if err != nil {
		t.Fatalf("run environment command: %v", err)
	}
	if output != "project-alpha\n" {
		t.Fatalf("project ID = %q, want fixture value", output)
	}
}

func TestWriteFileRejectsPathEscape(t *testing.T) {
	harness := NewHarness(t, "project", "session")
	if _, err := harness.ResolvePath("../outside"); err == nil {
		t.Fatal("path escape was accepted")
	}
}

func TestHarnessProvidesIndependentProjectDatabasePaths(t *testing.T) {
	first := NewHarness(t, "project-alpha", "session-one")
	second := NewHarness(t, "project-beta", "session-two")

	if first.DatabasePath == "" {
		t.Fatal("first harness database path is empty")
	}
	if second.DatabasePath == "" {
		t.Fatal("second harness database path is empty")
	}
	if first.DatabasePath == second.DatabasePath {
		t.Fatal("harnesses must use different project database paths")
	}
	if got := environmentValue(first.Environment, "JACAZUL_FLOW_DATABASE_PATH"); got != first.DatabasePath {
		t.Fatalf("first harness database environment = %q, want %q", got, first.DatabasePath)
	}
	if got := environmentValue(second.Environment, "JACAZUL_FLOW_DATABASE_PATH"); got != second.DatabasePath {
		t.Fatalf("second harness database environment = %q, want %q", got, second.DatabasePath)
	}

	if err := os.WriteFile(first.DatabasePath, []byte("alpha"), 0o600); err != nil {
		t.Fatalf("write first database fixture: %v", err)
	}
	if _, err := os.Stat(second.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("second project database unexpectedly exists: %v", err)
	}
}

func environmentValue(environment []string, key string) string {
	prefix := key + "="
	for _, entry := range environment {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			return entry[len(prefix):]
		}
	}
	return ""
}
