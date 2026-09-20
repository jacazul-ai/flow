package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
)

// TicketCommand links a task to an external ticket reference.
type TicketCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *TicketCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute stores one direct ticket reference on a task.
func (cmd *TicketCommand) Execute(args []string) error {
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		return fmt.Errorf("ticket requires a task UUID and ticket reference\nACTION: Run 'jczl-flow ticket <uuid> <ticket>'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.SetTaskTicket(context.Background(), args[0], args[1]); err != nil {
		return err
	}
	current, err := store.GetTask(context.Background(), args[0])
	if err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Linked ticket %s to task %s\n", strings.TrimSpace(args[1]), shortID(current.ID))
	return nil
}
