package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// OnboardCommand renders one deterministic agent context briefing.
type OnboardCommand struct {
	ReportFormat
	appOpts *config.AppOptions
}

// SetAppOptions supplies project and session options to the onboard command.
func (cmd *OnboardCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders handoff, focus, context, and the appropriate workflow view.
func (cmd *OnboardCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("onboard accepts no arguments")
	}
	format, err := cmd.resolve(cmd.appOpts)
	if err != nil {
		return err
	}

	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := cmd.appOpts.Context()
	focus, err := store.LoadFocus(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	note, found, err := store.GetSessionNote(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}

	if format != formatText {
		return cmd.report(store, format, focus, note, found)
	}

	output, acknowledge, err := renderOnboard(ctx, store, cmd.appOpts, focus, note, found)
	if err != nil {
		return err
	}
	if acknowledge {
		if _, _, err := store.AcknowledgeSessionNote(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID); err != nil {
			return fmt.Errorf("acknowledge onboard handoff: %w", err)
		}
		output += "HANDOFF ACKNOWLEDGED\n"
	}

	fmt.Fprint(cmd.appOpts.Out(), output)
	return nil
}

// report renders the onboard briefing as one record. A pending handoff is
// carried in full and acknowledged exactly as the text briefing does.
func (cmd *OnboardCommand) report(store *sqlite.Store, format string, focus task.FocusState, note task.SessionNote, noteFound bool) error {
	ctx := cmd.appOpts.Context()
	acknowledge := noteFound && note.AcknowledgedAt == ""
	handoff := ""
	if acknowledge {
		handoff = note.Content
	}
	focusedName, err := onboardFocusedInitiative(ctx, store, cmd.appOpts, focus)
	if err != nil {
		return err
	}
	contextRecs, err := onboardContextRecords(ctx, store, focus)
	if err != nil {
		return err
	}
	tasks, initiatives, err := onboardWorkRecords(ctx, store, cmd.appOpts, focus, focusedName)
	if err != nil {
		return err
	}
	if acknowledge {
		if _, _, err := store.AcknowledgeSessionNote(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID); err != nil {
			return fmt.Errorf("acknowledge onboard handoff: %w", err)
		}
	}
	return writeReport(cmd.appOpts, format, report{command: "onboard", records: []record{{
		{"handoff", handoff},
		{"handoff_acknowledged", acknowledge},
		{"focus_initiative", focusedName},
		{"focus_task_id", focus.FocusedTaskID},
		{"context", contextRecs},
		{"tasks", tasks},
		{"initiatives", initiatives},
	}}})
}

func onboardContextRecords(ctx context.Context, store *sqlite.Store, focus task.FocusState) ([]record, error) {
	if focus.FocusedTaskID == "" {
		return []record{}, nil
	}
	current, err := store.GetTask(ctx, focus.FocusedTaskID)
	if err != nil {
		return nil, err
	}
	direct, err := store.ListAnnotations(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	inherited, err := store.InheritedAnnotations(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	return contextRecords(current, direct, inherited), nil
}

// onboardWorkRecords mirrors the text briefing: the focused initiative's
// status when there is a focus, the ponder initiatives otherwise.
func onboardWorkRecords(ctx context.Context, store *sqlite.Store, opts *config.AppOptions, focus task.FocusState, focusedName string) ([]record, []record, error) {
	if focusedName != "" || focus.FocusedTaskID != "" {
		tasks, err := store.ListTasks(ctx, opts.ProjectID, focusedName)
		if err != nil {
			return nil, nil, err
		}
		records, err := taskRecords(ctx, store, statusTasks(tasks, false))
		return records, []record{}, err
	}
	summaries, err := store.ListInitiatives(ctx, opts.ProjectID, false, true)
	if err != nil {
		return nil, nil, err
	}
	visible, _ := visibleSummaries(summaries, focus, false)
	return []record{}, initiativeRecords(visible), nil
}

func renderOnboard(
	ctx context.Context,
	store *sqlite.Store,
	opts *config.AppOptions,
	focus task.FocusState,
	note task.SessionNote,
	noteFound bool,
) (string, bool, error) {
	var output strings.Builder
	output.WriteString("🚀 ONBOARD BRIEFING\n\n")

	acknowledge := noteFound && note.AcknowledgedAt == ""
	if acknowledge {
		output.WriteString("📋 SESSION HANDOFF (READ FIRST)\n\n")
		output.WriteString(strings.TrimRight(note.Content, "\n"))
		output.WriteString("\n\n")
	}

	focusedName, err := onboardFocusedInitiative(ctx, store, opts, focus)
	if err != nil {
		return "", false, err
	}
	appendSessionContext(&output, focus, focusedName)
	if err := appendOnboardFocusContext(ctx, store, focus, &output); err != nil {
		return "", false, err
	}

	if focusedName != "" || focus.FocusedTaskID != "" {
		tasks, err := store.ListTasks(ctx, opts.ProjectID, focusedName)
		if err != nil {
			return "", false, err
		}
		status, err := renderStatus(ctx, store, tasks, focusedName, focus.FocusedTaskID, false)
		if err != nil {
			return "", false, err
		}
		output.WriteString("STATUS:\n")
		output.WriteString(status)
		return output.String(), acknowledge, nil
	}

	summaries, err := store.ListInitiatives(ctx, opts.ProjectID, false, true)
	if err != nil {
		return "", false, err
	}
	dashboard, err := renderDashboard(ctx, store, opts, summaries, focus, false, false)
	if err != nil {
		return "", false, err
	}
	output.WriteString("PONDER:\n")
	output.WriteString(dashboard)
	return output.String(), acknowledge, nil
}

func onboardFocusedInitiative(
	ctx context.Context,
	store *sqlite.Store,
	opts *config.AppOptions,
	focus task.FocusState,
) (string, error) {
	if focus.InitiativeID != "" {
		initiative, err := store.FindInitiativeByID(ctx, opts.ProjectID, focus.InitiativeID)
		if err != nil {
			return "", err
		}
		return initiative.Name, nil
	}
	if focus.FocusedTaskID == "" {
		return "", nil
	}
	current, err := store.GetTask(ctx, focus.FocusedTaskID)
	if err != nil {
		return "", err
	}
	return current.InitiativeName, nil
}

func appendOnboardFocusContext(
	ctx context.Context,
	store *sqlite.Store,
	focus task.FocusState,
	output *strings.Builder,
) error {
	output.WriteString("FOCUS CONTEXT:\n")
	if focus.FocusedTaskID == "" {
		output.WriteString("No focused task.\n\n")
		return nil
	}

	current, err := store.GetTask(ctx, focus.FocusedTaskID)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "Task: %s %s\n", shortID(current.ID), current.Description)
	direct, err := store.ListAnnotations(ctx, current.ID)
	if err != nil {
		return err
	}
	inherited, err := store.InheritedAnnotations(ctx, current.ID)
	if err != nil {
		return err
	}
	writeDirectContext(output, direct)
	writeInheritedContext(output, inherited)
	if len(direct) == 0 && len(inherited) == 0 {
		output.WriteString("No context recorded.\n")
	}
	output.WriteString("\n")
	return nil
}
