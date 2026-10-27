package cli

import (
	"fmt"

	"github.com/jacazul-ai/flow/internal/config"
)

// InitCommand creates or upgrades the project database without touching
// initiatives, tasks, sessions, focus, or cache state.
type InitCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies resolved runtime options to the command.
func (cmd *InitCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute opens the project database and reports what opening it changed.
func (cmd *InitCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("init accepts no arguments\nACTION: Run 'jczl-flow init', or 'jczl-flow help init' for its options.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}

	info := store.MigrationInfo()
	path := cmd.appOpts.DatabasePath
	out := cmd.appOpts.Out()
	switch {
	case info.Created:
		fmt.Fprintf(out, "Created project database at %s (schema v%d)\n", path, info.CurrentVersion)
	case info.UpgradeRequired:
		fmt.Fprintf(out, "Upgraded project database at %s (v%d -> v%d)\n", path, info.PreviousVersion, info.CurrentVersion)
	case cmd.appOpts.Verbose:
		fmt.Fprintf(out, "Project database is current at %s (schema v%d)\n", path, info.CurrentVersion)
	}
	return store.Close()
}
