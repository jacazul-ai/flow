package cli

import (
	"testing"

	"github.com/jessevdk/go-flags"
)

func TestCommandRegistryExposesCanonicalAliases(t *testing.T) {
	registry := NewCommandRegistry()

	plan, ok := registry.Find("plan")
	if !ok {
		t.Fatal("plan is missing from command registry")
	}
	if plan.HandlerFactory == nil {
		t.Fatal("plan has no handler factory")
	}
	if got, want := plan.Aliases, []string{"initiative", "ini"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("plan aliases = %v, want %v", got, want)
	}

	for _, alias := range []string{"initiative", "ini", "inis", "initiatives", "ship"} {
		spec, ok := registry.Find(alias)
		if !ok {
			t.Fatalf("alias %q is missing from command registry", alias)
		}
		if !spec.Hidden {
			t.Fatalf("alias %q is visible in the command registry", alias)
		}
	}
}

func TestCommandRegistryCommonGroupsExcludeHiddenEntries(t *testing.T) {
	registry := NewCommandRegistry()
	for _, group := range registry.CommonGroups() {
		for _, spec := range group.Commands {
			if spec.Hidden {
				t.Fatalf("hidden command %q is in common group %q", spec.Name, group.Name)
			}
		}
	}

	for _, spec := range registry.All(false) {
		if spec.Hidden {
			t.Fatalf("hidden command %q returned by All(false)", spec.Name)
		}
	}
	if len(registry.All(true)) <= len(registry.All(false)) {
		t.Fatal("All(true) did not include hidden entries")
	}
}

func TestRegisterCommandsUsesParserAliases(t *testing.T) {
	parser := flags.NewParser(&struct{}{}, flags.Default)
	if err := RegisterCommands(parser); err != nil {
		t.Fatalf("register commands: %v", err)
	}

	plan := parser.Command.Find("plan")
	if plan == nil {
		t.Fatal("registered parser is missing plan")
	}
	if got, want := plan.Aliases, []string{"initiative", "ini"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("parser plan aliases = %v, want %v", got, want)
	}
	if parser.Command.Find("initiative") != plan || parser.Command.Find("ini") != plan {
		t.Fatal("parser aliases do not resolve to the canonical plan command")
	}

	ship := parser.Command.Find("ship")
	if ship == nil || !ship.Hidden {
		t.Fatal("ship compatibility command should be registered as hidden")
	}
}
