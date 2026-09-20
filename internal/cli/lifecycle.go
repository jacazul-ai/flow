package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
)

// ExecuteCommand starts a ready task.
type ExecuteCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *ExecuteCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute starts one task after checking its dependency chain.
func (cmd *ExecuteCommand) Execute(args []string) error {
	taskID, err := oneTaskID("execute", args)
	if err != nil {
		return err
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	current, err := store.GetTask(context.Background(), taskID)
	if err != nil {
		return err
	}
	if err := store.StartTask(context.Background(), current.ID); err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Started task %s\n", shortID(current.ID))
	return nil
}

// OutcomeCommand records the required outcome for a task.
type OutcomeCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *OutcomeCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute records all remaining arguments as one outcome message.
func (cmd *OutcomeCommand) Execute(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("outcome requires a task UUID and message\nACTION: Run 'jczl-flow outcome <uuid> <message>'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	current, err := store.GetTask(context.Background(), args[0])
	if err != nil {
		return err
	}
	if err := store.RecordOutcome(context.Background(), current.ID, strings.Join(args[1:], " ")); err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Recorded outcome for task %s\n", shortID(current.ID))
	return nil
}

// DoneCommand completes a task after its outcome gate passes.
type DoneCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *DoneCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute completes one task and reports newly ready tasks in its initiative.
func (cmd *DoneCommand) Execute(args []string) error {
	taskID, err := oneTaskID("done", args)
	if err != nil {
		return err
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	current, err := store.GetTask(context.Background(), taskID)
	if err != nil {
		return err
	}
	if err := store.CompleteTask(context.Background(), current.ID); err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Completed task %s\n", shortID(current.ID))

	ready, err := store.ReadyTasks(context.Background(), cmd.appOpts.ProjectID, current.InitiativeName)
	if err != nil {
		return fmt.Errorf("find next ready tasks: %w", err)
	}
	for _, next := range ready {
		fmt.Printf("Ready task %s: %s\n", shortID(next.ID), next.Description)
	}
	return nil
}

// ReopenCommand reopens a completed task.
type ReopenCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *ReopenCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute reopens one completed task.
func (cmd *ReopenCommand) Execute(args []string) error {
	taskID, err := oneTaskID("reopen", args)
	if err != nil {
		return err
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	current, err := store.GetTask(context.Background(), taskID)
	if err != nil {
		return err
	}
	if err := store.ReopenTask(context.Background(), current.ID); err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Reopened task %s\n", shortID(current.ID))
	return nil
}

// DiscardCommand discards a task with an audit outcome.
type DiscardCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *DiscardCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute discards one task and records the audit outcome.
func (cmd *DiscardCommand) Execute(args []string) error {
	taskID, err := oneTaskID("discard", args)
	if err != nil {
		return err
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	current, err := store.GetTask(context.Background(), taskID)
	if err != nil {
		return err
	}
	if err := store.DiscardTask(context.Background(), current.ID); err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Printf("Discarded task %s\n", shortID(current.ID))
	return nil
}

func openStore(opts *config.AppOptions) (*sqlite.Store, error) {
	moved, err := sqlite.MoveLegacyDatabase(opts.LegacyDatabasePath, opts.DatabasePath)
	if err != nil {
		return nil, err
	}
	if moved {
		fmt.Fprintf(opts.Stdout, "Moved legacy database to %s\n", opts.DatabasePath)
	}

	store, err := sqlite.Open(context.Background(), opts.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open project database: %w", err)
	}
	return store, nil
}

func oneTaskID(command string, args []string) (string, error) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return "", fmt.Errorf("%s requires exactly one task UUID\nACTION: Run 'jczl-flow help %s'.", command, command)
	}
	return args[0], nil
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
