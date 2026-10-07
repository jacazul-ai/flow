package cli

import (
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
)

// GoalCommand sets the first-class goal of an initiative.
type GoalCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *GoalCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute stores all remaining arguments as one initiative goal.
func (cmd *GoalCommand) Execute(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("goal requires an initiative reference and message\nACTION: Run 'jczl-flow goal <initiative> <message>'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	initiative := strings.TrimSpace(args[0])
	goal := strings.TrimSpace(strings.Join(args[1:], " "))
	if err := store.SetInitiativeGoal(cmd.appOpts.Context(), cmd.appOpts.ProjectID, initiative, goal); err != nil {
		return err
	}
	if err := store.ClearCache(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID, ""); err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Set goal for initiative %s\n", initiative)
	return nil
}
