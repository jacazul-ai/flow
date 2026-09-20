package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/task"
)

// ActiveCommand lists tasks currently being executed.
type ActiveCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *ActiveCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders active tasks for the current project or initiative.
func (cmd *ActiveCommand) Execute(args []string) error {
	return listTaskView(cmd.appOpts, args, "ACTIVE TASKS", "No active tasks.", func(current task.Task, _ map[string]task.Status) bool {
		return current.Status == task.Active
	})
}

// BlockedCommand lists pending tasks with unfinished dependencies.
type BlockedCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *BlockedCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders blocked tasks for the current project or initiative.
func (cmd *BlockedCommand) Execute(args []string) error {
	return listTaskView(cmd.appOpts, args, "BLOCKED TASKS", "No blocked tasks.", func(current task.Task, states map[string]task.Status) bool {
		return current.Status == task.Pending && !dependenciesReady(current, states)
	})
}

// OverdueCommand lists pending tasks whose due date is before today.
type OverdueCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *OverdueCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders overdue tasks for the current project or initiative.
func (cmd *OverdueCommand) Execute(args []string) error {
	today := time.Now().UTC().Format("2006-01-02")
	return listTaskView(cmd.appOpts, args, "OVERDUE TASKS", "No overdue tasks.", func(current task.Task, _ map[string]task.Status) bool {
		return current.Status == task.Pending && current.DueAt != "" && current.DueAt < today
	})
}

func listTaskView(
	opts *config.AppOptions,
	args []string,
	header string,
	emptyMessage string,
	include func(task.Task, map[string]task.Status) bool,
) error {
	if len(args) > 1 {
		return fmt.Errorf("%s accepts at most one initiative name\nACTION: Run 'jczl-flow help %s'.", strings.ToLower(strings.TrimSuffix(header, " TASKS")), strings.ToLower(strings.TrimSuffix(header, " TASKS")))
	}
	initiativeName := ""
	if len(args) == 1 {
		initiativeName = args[0]
	}
	store, err := openStore(opts)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := opts.Context()
	tasks, err := store.ListTasks(ctx, opts.ProjectID, initiativeName)
	if err != nil {
		return err
	}
	states := make(map[string]task.Status, len(tasks))
	for _, current := range tasks {
		states[current.ID] = current.Status
	}

	var output strings.Builder
	output.WriteString(header + ":\n")
	for _, current := range tasks {
		if !include(current, states) {
			continue
		}
		line, err := formatStatusTask(ctx, store, current)
		if err != nil {
			return err
		}
		output.WriteString(line)
	}
	if output.Len() == len(header)+2 {
		output.WriteString(emptyMessage + "\n")
	}
	fmt.Fprint(opts.Out(), output.String())
	return nil
}
