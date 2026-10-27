package cli

import (
	"fmt"

	"github.com/jacazul-ai/flow/internal/config"
)

// InitResult describes what settling the project database changed.
type InitResult struct {
	Path       string
	LegacyPath string
	Moved      bool
	Created    bool
	Upgraded   bool
	From       int64
	To         int64
}

// InitStorage moves, creates, or upgrades the project database and closes it
// without touching initiatives, tasks, sessions, focus, or cache state. The
// move and upgrade notices go to the option streams, so a caller that must
// stay silent resolves them to io.Discard.
func InitStorage(opts *config.AppOptions) (InitResult, error) {
	store, moved, err := settleStore(opts)
	if err != nil {
		return InitResult{}, err
	}

	info := store.MigrationInfo()
	result := InitResult{
		Path:     opts.DatabasePath,
		Moved:    moved,
		Created:  info.Created,
		Upgraded: info.UpgradeRequired,
		From:     info.PreviousVersion,
		To:       info.CurrentVersion,
	}
	if moved {
		result.LegacyPath = opts.LegacyDatabasePath
	}
	return result, store.Close()
}

// InitCommand creates or upgrades the project database without touching
// initiatives, tasks, sessions, focus, or cache state.
type InitCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies resolved runtime options to the command.
func (cmd *InitCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute settles the project database and reports what that changed.
func (cmd *InitCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("init accepts no arguments\nACTION: Run 'jczl-flow init', or 'jczl-flow help init' for its options.")
	}
	result, err := InitStorage(cmd.appOpts)
	if err != nil {
		return err
	}

	out := cmd.appOpts.Out()
	switch {
	case result.Created:
		fmt.Fprintf(out, "Created project database at %s (schema v%d)\n", result.Path, result.To)
	case result.Upgraded:
		fmt.Fprintf(out, "Upgraded project database at %s (v%d -> v%d)\n", result.Path, result.From, result.To)
	case cmd.appOpts.Verbose:
		fmt.Fprintf(out, "Project database is current at %s (schema v%d)\n", result.Path, result.To)
	}
	return nil
}
