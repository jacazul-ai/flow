package cli

import (
	"context"
	"fmt"

	"github.com/jacazul-ai/flow/internal/config"
)

// NextCommand lists the next ready tasks for a project or initiative.
type NextCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *NextCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders pending tasks whose dependencies are complete.
func (cmd *NextCommand) Execute(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("next accepts at most one initiative name\nACTION: Run 'jczl-flow next [initiative]'.")
	}
	initiativeName := ""
	if len(args) == 1 {
		initiativeName = args[0]
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	ready, err := store.ReadyTasks(context.Background(), cmd.appOpts.ProjectID, initiativeName)
	if err != nil {
		return err
	}
	if len(ready) == 0 {
		fmt.Println("No tasks ready.")
		return nil
	}
	for _, current := range ready {
		fmt.Printf("%s %s\n", shortID(current.ID), current.Description)
	}
	return nil
}
