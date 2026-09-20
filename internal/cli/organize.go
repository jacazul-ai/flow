package cli

import (
	"context"
	"fmt"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// OrganizeCommand groups explicit task-ordering operations.
type OrganizeCommand struct {
	Order OrganizeOrderCommand `command:"order" description:"Reorder selected pending-task slots"`
	First OrganizeFirstCommand `command:"first" description:"Move one pending task to the first pending slot"`
	After OrganizeAfterCommand `command:"after" description:"Move one pending task after another"`
	Block OrganizeBlockCommand `command:"block" description:"Move an ordered pending-task block"`
}

// Execute requires one organization operation.
func (cmd *OrganizeCommand) Execute(args []string) error {
	return fmt.Errorf("organize requires an operation\nACTION: Run 'jczl-flow help organize'.")
}

// OrganizeOrderCommand permutes only the pending-task slots named by the user.
type OrganizeOrderCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped options to the command.
func (cmd *OrganizeOrderCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute reorders selected pending-task slots in the supplied initiative.
func (cmd *OrganizeOrderCommand) Execute(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("organize order requires an initiative and at least two task references\nACTION: Run 'jczl-flow organize order <initiative> <task> <task> [...].")
	}
	return organize(cmd.appOpts, args[0], args[1:], organizeSelectedSlots)
}

// OrganizeFirstCommand moves one pending task to the first pending slot.
type OrganizeFirstCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped options to the command.
func (cmd *OrganizeFirstCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute moves one task to the first pending slot.
func (cmd *OrganizeFirstCommand) Execute(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("organize first requires an initiative and task reference\nACTION: Run 'jczl-flow organize first <initiative> <task>'.")
	}
	return organize(cmd.appOpts, args[0], args[1:], organizeFirst)
}

// OrganizeAfterCommand moves one pending task after a pending anchor task.
type OrganizeAfterCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped options to the command.
func (cmd *OrganizeAfterCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute moves a task after one anchor.
func (cmd *OrganizeAfterCommand) Execute(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("organize after requires an initiative, task, and anchor\nACTION: Run 'jczl-flow organize after <initiative> <task> <anchor>'.")
	}
	return organizeAfterTask(cmd.appOpts, args[0], args[1], args[2])
}

// OrganizeBlockCommand moves an ordered pending-task block to a first or relative position.
type OrganizeBlockCommand struct {
	First   bool   `long:"first" description:"Move the block to the first pending slot"`
	After   string `long:"after" description:"Move the block after this pending task reference"`
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped options to the command.
func (cmd *OrganizeBlockCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute moves an ordered block of pending tasks.
func (cmd *OrganizeBlockCommand) Execute(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("organize block requires an initiative and at least two task references\nACTION: Run 'jczl-flow organize block <initiative> <task> <task> --first|--after <anchor>'.")
	}
	if cmd.First == (cmd.After != "") {
		return fmt.Errorf("organize block requires exactly one destination\nACTION: Use either --first or --after <anchor>.")
	}
	if cmd.First {
		return organize(cmd.appOpts, args[0], args[1:], organizeFirstBlock)
	}
	return organizeAfterBlock(cmd.appOpts, args[0], args[1:], cmd.After)
}

type organizeOperation func(current []string, selected []string) ([]string, error)

func organize(opts *config.AppOptions, initiativeRef string, taskRefs []string, operation organizeOperation) error {
	store, initiative, current, err := openOrganization(opts, initiativeRef)
	if err != nil {
		return err
	}
	defer store.Close()

	selected, err := resolvePendingOrganizationTasks(opts.Context(), store, initiative, taskRefs)
	if err != nil {
		return err
	}
	desired, err := operation(current, selected)
	if err != nil {
		return err
	}
	return persistOrganization(store, opts, initiative, current, desired)
}

func organizeAfterTask(opts *config.AppOptions, initiativeRef string, taskRef string, anchorRef string) error {
	return organizeAfterBlock(opts, initiativeRef, []string{taskRef}, anchorRef)
}

func organizeAfterBlock(opts *config.AppOptions, initiativeRef string, taskRefs []string, anchorRef string) error {
	store, initiative, current, err := openOrganization(opts, initiativeRef)
	if err != nil {
		return err
	}
	defer store.Close()

	selected, err := resolvePendingOrganizationTasks(opts.Context(), store, initiative, taskRefs)
	if err != nil {
		return err
	}
	anchor, err := resolvePendingOrganizationTasks(opts.Context(), store, initiative, []string{anchorRef})
	if err != nil {
		return err
	}
	if selectedContains(selected, anchor[0]) {
		return fmt.Errorf("organization anchor cannot be part of the moved block\nACTION: Choose a pending task outside the block as --after.")
	}
	desired := moveTaskBlockAfter(current, selected, anchor[0])
	return persistOrganization(store, opts, initiative, current, desired)
}

func openOrganization(opts *config.AppOptions, initiativeRef string) (*sqlite.Store, task.Initiative, []string, error) {
	store, err := openStore(opts)
	if err != nil {
		return nil, task.Initiative{}, nil, err
	}
	initiative, err := store.FindInitiativeReference(opts.Context(), opts.ProjectID, initiativeRef)
	if err != nil {
		store.Close()
		return nil, task.Initiative{}, nil, fmt.Errorf("resolve organization initiative: %w\nACTION: Use an initiative name or unambiguous UUID in the selected project.", err)
	}
	tasks, err := store.ListTasks(opts.Context(), opts.ProjectID, initiative.Name)
	if err != nil {
		store.Close()
		return nil, task.Initiative{}, nil, err
	}
	current := make([]string, 0, len(tasks))
	for _, currentTask := range tasks {
		if currentTask.Status == task.Pending {
			current = append(current, currentTask.ID)
		}
	}
	return store, initiative, current, nil
}

func resolvePendingOrganizationTasks(ctx context.Context, store *sqlite.Store, initiative task.Initiative, refs []string) ([]string, error) {
	selected := make([]string, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	for _, reference := range refs {
		current, err := store.GetTask(ctx, reference)
		if err != nil {
			return nil, fmt.Errorf("resolve organization task %q: %w\nACTION: Use a full or unambiguous task UUID in the selected initiative.", reference, err)
		}
		if current.InitiativeID != initiative.ID {
			return nil, fmt.Errorf("task %s does not belong to initiative %q\nACTION: Organize tasks only within one initiative.", shortID(current.ID), initiative.Name)
		}
		if current.Status == task.Completed {
			return nil, fmt.Errorf("task %s is completed and cannot be organized\nACTION: Choose pending tasks only.", shortID(current.ID))
		}
		if current.Status != task.Pending {
			return nil, fmt.Errorf("task %s is %s and cannot be organized\nACTION: Choose pending tasks only.", shortID(current.ID), current.Status)
		}
		if seen[current.ID] {
			return nil, fmt.Errorf("organization contains duplicate task %s\nACTION: List each task reference once.", shortID(current.ID))
		}
		seen[current.ID] = true
		selected = append(selected, current.ID)
	}
	return selected, nil
}

func organizeSelectedSlots(current []string, selected []string) ([]string, error) {
	selectedSet := taskIDSet(selected)
	result := append([]string(nil), current...)
	selectedIndex := 0
	for index, taskID := range current {
		if !selectedSet[taskID] {
			continue
		}
		result[index] = selected[selectedIndex]
		selectedIndex++
	}
	return result, nil
}

func organizeFirst(current []string, selected []string) ([]string, error) {
	return organizeFirstBlock(current, selected)
}

func organizeFirstBlock(current []string, selected []string) ([]string, error) {
	return append(append([]string(nil), selected...), withoutTaskIDs(current, taskIDSet(selected))...), nil
}

func moveTaskBlockAfter(current []string, selected []string, anchor string) []string {
	remaining := withoutTaskIDs(current, taskIDSet(selected))
	for index, taskID := range remaining {
		if taskID != anchor {
			continue
		}
		result := make([]string, 0, len(current))
		result = append(result, remaining[:index+1]...)
		result = append(result, selected...)
		return append(result, remaining[index+1:]...)
	}
	return current
}

func persistOrganization(store *sqlite.Store, opts *config.AppOptions, initiative task.Initiative, current []string, desired []string) error {
	if sameTaskIDs(current, desired) {
		return nil
	}
	if err := store.ReplacePendingTaskOrder(opts.Context(), initiative.ID, desired); err != nil {
		return err
	}
	if err := clearTaskCaches(store, opts, task.Task{InitiativeName: initiative.Name}); err != nil {
		return err
	}
	fmt.Fprintf(opts.Out(), "Organized %d pending tasks in initiative %s\n", len(desired), initiative.Name)
	return nil
}

func taskIDSet(taskIDs []string) map[string]bool {
	set := make(map[string]bool, len(taskIDs))
	for _, taskID := range taskIDs {
		set[taskID] = true
	}
	return set
}

func selectedContains(selected []string, taskID string) bool {
	return taskIDSet(selected)[taskID]
}

func withoutTaskIDs(taskIDs []string, removed map[string]bool) []string {
	remaining := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if !removed[taskID] {
			remaining = append(remaining, taskID)
		}
	}
	return remaining
}

func sameTaskIDs(first []string, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index, taskID := range first {
		if taskID != second[index] {
			return false
		}
	}
	return true
}
