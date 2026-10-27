package config_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jacazul-ai/flow/internal/config"
)

// The jacazul launcher rejects these global flags in its own argv, because it
// prepares storage from its resolved Env before the engine parses the
// operator's flags. Adding, renaming, or removing a value-taking global flag
// breaks the launcher's guard: update docs/cli.md and notify the launcher
// before changing this list.
var launcherGuardedFlags = []string{
	"database-path",
	"home",
	"project",
	"session",
	"task-binary",
	"taskdata",
}

func TestValueTakingGlobalFlagsMatchTheLauncherGuard(t *testing.T) {
	options := reflect.TypeOf(config.AppOptions{})
	var flags []string
	for i := range options.NumField() {
		field := options.Field(i)
		long := field.Tag.Get("long")
		if long == "" || field.Type.Kind() == reflect.Bool {
			continue
		}
		flags = append(flags, long)
	}
	slices.Sort(flags)

	if !slices.Equal(flags, launcherGuardedFlags) {
		t.Fatalf("value-taking global flags = %v, launcher guards %v\n"+
			"ACTION: Update docs/cli.md and notify the launcher before changing global flags.",
			flags, launcherGuardedFlags)
	}
}
