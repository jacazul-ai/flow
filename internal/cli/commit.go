package cli

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/task"
)

// CommitCommand renders a conventional commit draft without executing Git.
type CommitCommand struct {
	Fix     bool `long:"fix" description:"Use a Fixes footer when a ticket exists"`
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *CommitCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders the draft for the focused task.
func (cmd *CommitCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("commit accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	focus, err := store.LoadFocus(context.Background(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if focus.FocusedTaskID == "" {
		return fmt.Errorf("no focused task found\nACTION: Run 'jczl-flow focus task <uuid>' first.")
	}
	task, err := store.GetTask(context.Background(), focus.FocusedTaskID)
	if err != nil {
		return err
	}

	prefix := commitPrefix(task.Description, task.Mode)
	description := cleanDescription(task.Description)
	ticket, _, err := store.FindExternalTicket(context.Background(), task.ID)
	if err != nil {
		return err
	}
	fmt.Println("DRAFT CONVENTIONAL COMMIT")
	fmt.Printf("%s: %s\n", prefix, description)
	if ticket != "" {
		footer := "Refs"
		if cmd.Fix {
			footer = "Fixes"
		}
		fmt.Printf("\n%s: %s\n", footer, ticket)
	}
	fmt.Println("\nSAFETY: Draft only. Write a message file and obtain approval before git commit.")
	return nil
}

func commitPrefix(description string, mode task.TaskMode) string {
	if strings.Contains(description, "[DEBUG]") || strings.Contains(description, "[BUG]") {
		return "fix"
	}
	if strings.Contains(description, "[TEST]") || mode == task.ModeTest {
		return "test"
	}
	if strings.Contains(description, "[DESIGN]") || strings.Contains(description, "[SPIKE]") {
		return "docs"
	}
	if strings.Contains(description, "[REFINE]") || strings.Contains(description, "[REFACTOR]") {
		return "refactor"
	}
	return "feat"
}

func cleanDescription(description string) string {
	cleaned := regexp.MustCompile(`\[[A-Z-]+\]\s*`).ReplaceAllString(description, "")
	return strings.ToLower(strings.TrimSpace(cleaned))
}
