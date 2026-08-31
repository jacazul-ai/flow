package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jacazul-ai/jaflow/internal/testharness"
)

func TestRootHelpGroupsCanonicalCommandsByIntent(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	output, err := runJaflow(t, binary, harness, "help")
	if err != nil {
		t.Fatalf("root help: %v\n%s", err, output)
	}

	groups := []struct {
		name     string
		commands []string
	}{
		{
			name: "start and organize work",
			commands: []string{
				"plan", "roadmap", "rename", "backlog", "activate",
			},
		},
		{
			name: "examine workflow state",
			commands: []string{
				"help", "status", "ponder", "plans", "next", "history", "tree",
				"active", "blocked", "overdue",
			},
		},
		{
			name: "work on the current task",
			commands: []string{
				"focus", "execute", "outcome", "done", "reopen", "discard",
				"handoff",
			},
		},
		{
			name: "change and reprioritize work",
			commands: []string{
				"amend", "urgent", "block", "unblock", "wait",
			},
		},
		{
			name:     "preserve and maintain context",
			commands: []string{"session", "note", "notes", "context", "ticket"},
		},
		{
			name:     "maintain derived workflow state",
			commands: []string{"cache"},
		},
		{
			name:     "prepare and integrate changes",
			commands: []string{"commit"},
		},
		{
			name:     "migrate legacy state",
			commands: []string{"migrate"},
		},
	}

	positions := make([]int, 0, len(groups))
	for _, group := range groups {
		position := strings.Index(output, group.name)
		if position < 0 {
			t.Fatalf("root help = %q, missing group %q", output, group.name)
		}
		positions = append(positions, position)
	}
	for index := 1; index < len(positions); index++ {
		if positions[index] <= positions[index-1] {
			t.Fatalf("root help group order = %v, want deterministic order", positions)
		}
	}

	commandLine := regexp.MustCompile(`(?m)^  ([a-z-]+)\s{2,}`)
	counts := make(map[string]int)
	for _, match := range commandLine.FindAllStringSubmatch(output, -1) {
		counts[match[1]]++
	}
	for _, group := range groups {
		for _, command := range group.commands {
			if counts[command] != 1 {
				t.Fatalf("root help command %q count = %d, want one canonical entry", command, counts[command])
			}
		}
	}
	for _, alias := range []string{"initiative", "ini", "inis", "initiatives", "ship"} {
		if counts[alias] != 0 {
			t.Fatalf("root help exposes alias %q %d times", alias, counts[alias])
		}
	}
}

func TestEmptyInvocationMatchesHelpCommandExactly(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")

	expected, err := runJaflow(t, binary, harness, "help")
	if err != nil {
		t.Fatalf("explicit help: %v\n%s", err, expected)
	}
	actual, err := runJaflow(t, binary, harness)
	if err != nil {
		t.Fatalf("empty invocation: %v\n%s", err, actual)
	}
	if actual != expected {
		t.Fatalf("empty invocation differs from help\nempty:\n%s\nhelp:\n%s", actual, expected)
	}
}

func TestHelpAliasesRemainDetailedAndRoutable(t *testing.T) {
	binary := buildJaflow(t)
	harness := testharness.NewHarness(t, "project", "session")
	for _, alias := range []string{"initiative", "ini", "inis", "initiatives", "ship"} {
		output, err := runJaflow(t, binary, harness, "help", alias)
		if err != nil {
			t.Fatalf("help %s: %v\n%s", alias, err, output)
		}
		if !strings.Contains(output, "USAGE") || !strings.Contains(output, "NEXT ACTION") {
			t.Fatalf("help %s = %q, want detailed help", alias, output)
		}
	}
}
