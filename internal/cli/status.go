package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// StatusCommand displays project task state.
type StatusCommand struct {
	PendingOnly bool `long:"pending" description:"Show pending tasks only"`
	Force       bool `long:"force" description:"Bypass the status cache"`
	appOpts     *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *StatusCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute prints tasks, optionally filtered by initiative name.
func (cmd *StatusCommand) Execute(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("status accepts at most one initiative name")
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

	cacheKey := "status"
	if initiativeName != "" {
		cacheKey += "_" + initiativeName
	}
	if !cmd.Force && !cmd.PendingOnly {
		_, found, err := store.GetCache(
			context.Background(),
			cmd.appOpts.ProjectID,
			cmd.appOpts.SessionID,
			cacheKey,
			time.Now().UTC(),
		)
		if err != nil {
			return err
		}
		if found {
			fmt.Fprintln(cmd.appOpts.Out(), "🐊 [cached] Status unchanged. Use --force to refresh.")
			return nil
		}
	}

	tasks, err := store.ListTasks(context.Background(), cmd.appOpts.ProjectID, initiativeName)
	if err != nil {
		return err
	}
	focus, err := store.LoadFocus(context.Background(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	output, err := renderStatus(
		context.Background(), store, tasks, initiativeName, focus.FocusedTaskID, cmd.PendingOnly,
	)
	if err != nil {
		return err
	}
	fmt.Fprint(cmd.appOpts.Out(), output)
	if !cmd.PendingOnly {
		if err := store.SetCache(
			context.Background(),
			cmd.appOpts.ProjectID,
			cmd.appOpts.SessionID,
			cacheKey,
			output,
			time.Now().UTC().Add(2*time.Minute),
		); err != nil {
			return err
		}
	}
	return nil
}

func renderStatus(
	ctx context.Context,
	store *sqlite.Store,
	tasks []task.Task,
	initiativeName string,
	focusedTaskID string,
	pendingOnly bool,
) (string, error) {
	var output strings.Builder
	if initiativeName == "" {
		initiativeName = "ALL ACTIVE"
	}
	fmt.Fprintf(&output, "══ Plan: %s ══\n", initiativeName)
	if len(tasks) == 0 {
		output.WriteString("No tasks found.\n")
		return output.String(), nil
	}

	if focusedTaskID != "" {
		if err := appendFocusedContext(ctx, store, tasks, focusedTaskID, &output); err != nil {
			return "", err
		}
	}

	pending := 0
	completed := 0
	for _, current := range tasks {
		switch current.Status {
		case task.Pending, task.Active:
			pending++
		case task.Completed:
			completed++
		}
	}
	if pending > 0 {
		output.WriteString("PENDING:\n")
		for _, current := range tasks {
			if current.Status != task.Pending && current.Status != task.Active {
				continue
			}
			line, err := formatStatusTask(ctx, store, current)
			if err != nil {
				return "", err
			}
			output.WriteString(line)
		}
	}
	if !pendingOnly && completed > 0 {
		output.WriteString("COMPLETED:\n")
		for _, current := range tasks {
			if current.Status != task.Completed {
				continue
			}
			line, err := formatStatusTask(ctx, store, current)
			if err != nil {
				return "", err
			}
			output.WriteString(line)
		}
	}
	if output.Len() == 0 {
		return "No tasks found.\n", nil
	}
	return output.String(), nil
}

func formatStatusTask(ctx context.Context, store *sqlite.Store, current task.Task) (string, error) {
	ticket, _, err := store.FindExternalTicket(ctx, current.ID)
	if err != nil {
		return "", err
	}
	line := fmt.Sprintf("- [%s]", shortID(current.ID))
	if ticket != "" {
		line += " [" + ticket + "]"
	}
	return fmt.Sprintf("%s %s\n", line, current.Description), nil
}

func appendFocusedContext(
	ctx context.Context,
	store *sqlite.Store,
	tasks []task.Task,
	focusedTaskID string,
	output *strings.Builder,
) error {
	focused, err := store.GetTask(ctx, focusedTaskID)
	if err != nil {
		return err
	}
	for _, current := range tasks {
		if current.ID != focused.ID {
			continue
		}
		ticket, inheritedTicket, err := store.FindExternalTicket(ctx, focused.ID)
		if err != nil {
			return err
		}
		if ticket != "" {
			kind := "External"
			if inheritedTicket {
				kind = "Inherited"
			}
			fmt.Fprintf(output, "🐊 ALERT: %s ticket detected (%s)\n", kind, ticket)
		}
		direct, err := store.ListAnnotations(ctx, focused.ID)
		if err != nil {
			return err
		}
		inherited, err := store.InheritedAnnotations(ctx, focused.ID)
		if err != nil {
			return err
		}
		if len(direct) == 0 && len(inherited) == 0 {
			return nil
		}
		fmt.Fprintf(output, "FOCUS CONTEXT [%s]:\n", shortID(focused.ID))
		writeDirectContext(output, direct)
		writeInheritedContext(output, inherited)
		return nil
	}
	return nil
}
