package cli

import (
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/task"
)

// HandoffCommand starts a task and records its handoff context.
type HandoffCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *HandoffCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute starts the target task when needed and adds a HANDOFF annotation.
func (cmd *HandoffCommand) Execute(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("handoff requires a task UUID and message\nACTION: Run 'jczl-flow help handoff'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := cmd.appOpts.Context()
	current, err := store.GetTask(ctx, args[0])
	if err != nil {
		return err
	}
	if current.Status == task.Completed {
		return fmt.Errorf("task %s is already COMPLETED\nACTION: Reopen the task before handing it off.", shortID(current.ID))
	}
	message := strings.TrimSpace(strings.Join(args[1:], " "))
	if message == "" {
		return fmt.Errorf("handoff message cannot be empty\nACTION: Provide context after the task UUID.")
	}
	if err := store.StartTask(ctx, current.ID); err != nil {
		return err
	}
	if err := store.AddAnnotation(ctx, current.ID, "HANDOFF", message); err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Handoff to task %s with note\n", shortID(current.ID))
	return nil
}
