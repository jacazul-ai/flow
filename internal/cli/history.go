package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/jacazul-ai/jaflow/internal/config"
	"github.com/jacazul-ai/jaflow/internal/task"
)

// HistoryCommand renders immutable history for a task or initiative.
type HistoryCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *HistoryCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute reads task history or initiative history without mutating state.
func (cmd *HistoryCommand) Execute(args []string) error {
	if len(args) != 1 && len(args) != 2 {
		return fmt.Errorf("history requires a task UUID or initiative name\nACTION: Run 'jaflow history <uuid>' or 'jaflow history initiative <name>'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	if len(args) == 1 {
		events, err := store.ListHistory(ctx, args[0])
		if err != nil {
			return err
		}
		return renderHistory("task "+shortID(args[0]), events)
	}
	if args[0] != "initiative" && args[0] != "ini" && args[0] != "plan" {
		return fmt.Errorf("unknown history scope %q\nACTION: Use task or initiative.", args[0])
	}
	events, err := store.ListInitiativeHistory(ctx, cmd.appOpts.ProjectID, args[1])
	if err != nil {
		return err
	}
	return renderHistory("initiative "+args[1], events)
}

func renderHistory(subject string, events []task.HistoryEvent) error {
	fmt.Printf("HISTORY: %s\n", subject)
	if len(events) == 0 {
		fmt.Println("No history recorded.")
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
		fmt.Println(line)
	}
	return nil
}

func quoteHistoryValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<empty>"
	}
	return fmt.Sprintf("%q", value)
}
