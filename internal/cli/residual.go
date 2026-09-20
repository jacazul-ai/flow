package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/task"
)

// AmendCommand updates task description and ticket metadata.
type AmendCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *AmendCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute applies description= and ticket= updates to one task.
func (cmd *AmendCommand) Execute(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("amend requires a task UUID and metadata\nACTION: Use description=\"...\" or ticket=\"...\".")
	}
	update := task.TaskMetadataUpdate{}
	for _, raw := range args[1:] {
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			continue
		}
		switch key {
		case "description":
			update.Description = &value
		case "ticket":
			update.ExternalTicket = &value
		}
	}
	if update.Description == nil && update.ExternalTicket == nil {
		return fmt.Errorf("no valid fields to amend\nACTION: Use description=\"...\" or ticket=\"...\".")
	}

	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	current, err := store.UpdateTaskMetadata(context.Background(), args[0], update)
	if err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Amended task %s metadata\n", shortID(current.ID))
	return nil
}

// RenameCommand renames an initiative and preserves its task identity.
type RenameCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *RenameCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renames one initiative.
func (cmd *RenameCommand) Execute(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("rename requires old and new initiative names\nACTION: Run 'jaflow rename <old-name> <new-name>'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.RenameInitiative(context.Background(), cmd.appOpts.ProjectID, args[0], args[1]); err != nil {
		return err
	}
	if err := store.ClearCache(context.Background(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID, ""); err != nil {
		return err
	}
	fmt.Printf("Renamed initiative %s to %s\n", args[0], args[1])
	return nil
}

// UrgentCommand raises task priority and urgency.
type UrgentCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *UrgentCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute marks a task urgent with an optional urgency value.
func (cmd *UrgentCommand) Execute(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("urgent requires a task UUID and optional urgency\nACTION: Run 'jaflow urgent <uuid> [urgency]'.")
	}
	urgency := 15.0
	if len(args) == 2 {
		parsed, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			return fmt.Errorf("invalid urgency %q: %w", args[1], err)
		}
		urgency = parsed
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.SetTaskUrgency(context.Background(), args[0], urgency); err != nil {
		return err
	}
	current, err := store.GetTask(context.Background(), args[0])
	if err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Task %s marked urgent (urgency: %.1f)\n", shortID(current.ID), urgency)
	return nil
}

// BlockCommand adds a dependency to a task.
type BlockCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *BlockCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute makes the first task depend on the second task.
func (cmd *BlockCommand) Execute(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("block requires a task UUID and dependency UUID\nACTION: Run 'jaflow block <uuid> <dependency-uuid>'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.AddDependency(context.Background(), args[0], args[1]); err != nil {
		return err
	}
	current, err := store.GetTask(context.Background(), args[0])
	if err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Task %s now depends on task %s\n", shortID(current.ID), shortID(args[1]))
	return nil
}

// UnblockCommand removes a dependency from a task.
type UnblockCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *UnblockCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute removes one dependency edge.
func (cmd *UnblockCommand) Execute(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("unblock requires a task UUID and dependency UUID\nACTION: Run 'jaflow unblock <uuid> <dependency-uuid>'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.RemoveDependency(context.Background(), args[0], args[1]); err != nil {
		return err
	}
	current, err := store.GetTask(context.Background(), args[0])
	if err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Removed dependency from task %s\n", shortID(current.ID))
	return nil
}

// WaitCommand postpones task readiness until a date.
type WaitCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *WaitCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute stores a normalized wait-until date.
func (cmd *WaitCommand) Execute(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("wait requires a task UUID and date\nACTION: Run 'jaflow wait <uuid> <YYYY-MM-DD|today|tomorrow>'.")
	}
	waitUntil, err := normalizeDueDate(args[1])
	if err != nil {
		return err
	}
	if waitUntil == "" {
		return fmt.Errorf("wait date cannot be empty\nACTION: Provide YYYY-MM-DD, today, or tomorrow.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.SetTaskWait(context.Background(), args[0], waitUntil); err != nil {
		return err
	}
	current, err := store.GetTask(context.Background(), args[0])
	if err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Task %s waiting until %s\n", shortID(current.ID), waitUntil)
	return nil
}
