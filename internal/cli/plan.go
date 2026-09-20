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

// PlanCommand creates a plan and its initial tasks.
type PlanCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *PlanCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute creates one pending task for each description after the plan name.
func (cmd *PlanCommand) Execute(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("plan requires a name and at least one task")
	}

	store, err := sqlite.Open(context.Background(), cmd.appOpts.DatabasePath)
	if err != nil {
		return fmt.Errorf("open project database: %w", err)
	}
	defer store.Close()

	initiative, err := store.GetOrCreateInitiative(context.Background(), task.CreateInitiativeInput{
		ProjectID: cmd.appOpts.ProjectID,
		Name:      args[0],
	})
	if err != nil {
		return err
	}

	var previous string
	for _, rawDescription := range args[1:] {
		description, mode, dueAt, err := parseTaskSpec(rawDescription)
		if err != nil {
			return err
		}
		var dependencies []string
		if previous != "" {
			dependencies = []string{previous}
		}
		created, err := store.CreateTask(context.Background(), task.CreateTaskInput{
			InitiativeID: initiative.ID,
			Description:  description,
			Mode:         mode,
			DueAt:        dueAt,
			Dependencies: dependencies,
		})
		if err != nil {
			return err
		}
		fmt.Printf("Created task %s: %s\n", created.ID[:8], created.Description)
		previous = created.ID
	}
	if err := clearTaskCaches(store, cmd.appOpts, task.Task{InitiativeName: initiative.Name}); err != nil {
		return err
	}
	return nil
}

func parseTaskSpec(raw string) (description string, mode task.TaskMode, dueAt string, err error) {
	parts := strings.Split(raw, "|")
	if len(parts) == 1 {
		if strings.TrimSpace(parts[0]) == "" {
			return "", task.ModeUnspecified, "", fmt.Errorf("task description cannot be empty")
		}
		return strings.TrimSpace(parts[0]), task.ModeUnspecified, "", nil
	}

	if isInteractionMode(parts[0]) {
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return "", task.ModeUnspecified, "", fmt.Errorf("task description is required in %q", raw)
		}
		description = strings.TrimSpace(parts[1])
		mode, err = task.ParseTaskMode(parts[0])
		if len(parts) >= 4 {
			dueAt = parts[3]
		}
	} else {
		description = strings.TrimSpace(parts[0])
		if len(parts) >= 3 {
			dueAt = parts[2]
		}
	}
	if description == "" {
		return "", task.ModeUnspecified, "", fmt.Errorf("task description cannot be empty")
	}
	dueAt, err = normalizeDueDate(dueAt)
	if err != nil {
		return "", task.ModeUnspecified, "", err
	}
	return description, mode, dueAt, nil
}

func isInteractionMode(value string) bool {
	mode, err := task.ParseTaskMode(value)
	return err == nil && mode != task.ModeUnspecified
}

func normalizeDueDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	switch strings.ToLower(value) {
	case "yesterday":
		return today.AddDate(0, 0, -1).Format("2006-01-02"), nil
	case "today":
		return today.Format("2006-01-02"), nil
	case "tomorrow":
		return today.AddDate(0, 0, 1).Format("2006-01-02"), nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", fmt.Errorf("invalid due date %q; use YYYY-MM-DD, today, or tomorrow", value)
	}
	return parsed.Format("2006-01-02"), nil
}
