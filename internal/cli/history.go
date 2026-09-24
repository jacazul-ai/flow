package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/task"
)

// HistoryCommand renders immutable history for a task or initiative.
type HistoryCommand struct {
	ReportFormat
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *HistoryCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute reads task history or initiative history without mutating state.
func (cmd *HistoryCommand) Execute(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("history requires an explicit scope and reference\nACTION: Run 'jczl-flow history task <uuid>' or 'jczl-flow history initiative <reference>'.")
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
	switch args[0] {
	case "task":
		current, err := store.GetTask(ctx, args[1])
		if err != nil {
			return err
		}
		events, err := store.ListHistory(ctx, current.ID)
		if err != nil {
			return err
		}
		if format != formatText {
			return writeReport(cmd.appOpts, format, report{command: "history", records: historyRecords(events)})
		}
		return renderHistory(cmd.appOpts.Out(), "task "+shortID(current.ID), events)
	case "initiative", "ini", "plan":
		initiative, err := store.FindInitiativeReference(ctx, cmd.appOpts.ProjectID, args[1])
		if err != nil {
			return err
		}
		events, err := store.ListInitiativeHistory(ctx, cmd.appOpts.ProjectID, initiative.ID)
		if err != nil {
			return err
		}
		if format != formatText {
			return writeReport(cmd.appOpts, format, report{command: "history", records: historyRecords(events)})
		}
		return renderHistory(cmd.appOpts.Out(), "initiative "+initiative.Name+" [id:"+shortID(initiative.ID)+"]", events)
	default:
		return fmt.Errorf("unknown history scope %q\nACTION: Use task or initiative.", args[0])
	}
}

func renderHistory(out io.Writer, subject string, events []task.HistoryEvent) error {
	fmt.Fprintf(out, "HISTORY: %s\n", subject)
	if len(events) == 0 {
		fmt.Fprintln(out, "No history recorded.")
		return nil
	}
	for _, event := range events {
		line := fmt.Sprintf("[%s] %s", event.OccurredAt, event.EventType)
		if event.Property != "" {
			line += " " + event.Property
			if event.OldValue != "" || event.NewValue != "" {
				line += ": " + quoteHistoryValue(event.OldValue) + " -> " + quoteHistoryValue(event.NewValue)
			}
		}
		if event.Source != "" && event.Source != "native" {
			line += " (source: " + event.Source + ")"
		}
		fmt.Fprintln(out, line)
	}
	return nil
}

func historyRecords(events []task.HistoryEvent) []record {
	records := make([]record, 0, len(events))
	for _, event := range events {
		records = append(records, record{
			{"occurred_at", event.OccurredAt},
			{"event_type", event.EventType},
			{"property", event.Property},
			{"old_value", event.OldValue},
			{"new_value", event.NewValue},
			{"task_id", event.TaskID},
			{"initiative_id", event.InitiativeID},
			{"source", event.Source},
		})
	}
	return records
}

func quoteHistoryValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<empty>"
	}
	return fmt.Sprintf("%q", value)
}
