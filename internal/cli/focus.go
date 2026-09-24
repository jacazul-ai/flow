package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/task"
)

// FocusCommand groups focus navigation commands.
type FocusCommand struct {
	Show     FocusShowCommand        `command:"show" description:"Show the current focus"`
	Plan     FocusPlanCommand        `command:"plan" description:"Focus an initiative"`
	Task     FocusTaskCommand        `command:"task" description:"Focus a task"`
	Pop      FocusPopCommand         `command:"pop" description:"Pop the current task focus"`
	Clear    FocusClearCommand       `command:"clear" description:"Clear the session focus"`
	Back     FocusBackCommand        `command:"back" description:"Return to global focus"`
	Ind      FocusIndependentCommand `command:"ind" description:"Use independent session focus"`
	Ini      FocusPlanCommand        `command:"ini" description:"Focus an initiative"`
	Interest FocusInterestCommand    `command:"interest" description:"Manage plans of interest"`
	Args     []string                `positional-args:"yes"`
	appOpts  *config.AppOptions
}

// SetAppOptions supplies project options to all focus subcommands.
func (cmd *FocusCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
	cmd.Show.SetAppOptions(opts)
	cmd.Plan.SetAppOptions(opts)
	cmd.Task.SetAppOptions(opts)
	cmd.Pop.SetAppOptions(opts)
	cmd.Clear.SetAppOptions(opts)
	cmd.Back.SetAppOptions(opts)
	cmd.Ind.SetAppOptions(opts)
	cmd.Ini.SetAppOptions(opts)
	cmd.Interest.SetAppOptions(opts)
}

// Execute shows focus by default or supports smart initiative focus.
func (cmd *FocusCommand) Execute(args []string) error {
	if len(args) == 0 {
		return cmd.Show.Execute(nil)
	}
	if len(args) == 1 {
		return cmd.Plan.execute(args, false)
	}
	return fmt.Errorf("focus accepts at most one smart initiative name\nACTION: Run 'jczl-flow help focus'.")
}

// FocusIndependentCommand provides focus operations for an isolated session.
type FocusIndependentCommand struct {
	Plan    FocusPlanCommand `command:"plan" description:"Focus an initiative independently"`
	Task    FocusTaskCommand `command:"task" description:"Focus a task independently"`
	Args    []string         `positional-args:"yes"`
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to independent focus commands.
func (cmd *FocusIndependentCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
	cmd.Plan.SetAppOptions(opts)
	cmd.Task.SetAppOptions(opts)
}

// Execute supports smart independent focus by initiative name.
func (cmd *FocusIndependentCommand) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("focus ind requires plan or task\nACTION: Run 'jczl-flow focus ind plan <name>' or 'jczl-flow focus ind task <uuid>'.")
	}
	return cmd.Plan.execute(args, true)
}

// FocusInterestCommand manages the initiatives shown by dashboard views.
type FocusInterestCommand struct {
	Args    []string `positional-args:"yes"`
	appOpts *config.AppOptions
}

// SetAppOptions supplies project-scoped runtime options to the command.
func (cmd *FocusInterestCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute adds, removes, or lists plans of interest for the current session.
func (cmd *FocusInterestCommand) Execute(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("focus interest requires add, remove, or list\nACTION: Run 'jczl-flow help focus'.")
	}
	if args[0] != "list" && len(args) != 2 {
		return fmt.Errorf("focus interest %s requires an initiative name\nACTION: Run 'jczl-flow focus interest [add|remove] <initiative>'.", args[0])
	}
	if args[0] == "list" && len(args) != 1 {
		return fmt.Errorf("focus interest list accepts no arguments")
	}
	if args[0] != "add" && args[0] != "remove" && args[0] != "list" {
		return fmt.Errorf("unknown focus interest action %q\nACTION: Use add, remove, or list.", args[0])
	}

	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx := cmd.appOpts.Context()
	state, err := store.LoadFocus(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if args[0] == "list" {
		fmt.Fprintln(cmd.appOpts.Out(), "PLANS OF INTEREST:")
		for _, name := range state.PlansOfInterest {
			fmt.Fprintln(cmd.appOpts.Out(), name)
		}
		if len(state.PlansOfInterest) == 0 {
			fmt.Fprintln(cmd.appOpts.Out(), "(empty)")
		}
		return nil
	}

	name := strings.TrimSpace(args[1])
	if name == "" {
		return fmt.Errorf("initiative name cannot be empty")
	}
	if args[0] == "add" {
		for _, current := range state.PlansOfInterest {
			if current == name {
				fmt.Fprintf(cmd.appOpts.Out(), "Plan already in interests: %s\n", name)
				return nil
			}
		}
		state.PlansOfInterest = append(state.PlansOfInterest, name)
		sort.Strings(state.PlansOfInterest)
	} else {
		filtered := state.PlansOfInterest[:0]
		for _, current := range state.PlansOfInterest {
			if current != name {
				filtered = append(filtered, current)
			}
		}
		state.PlansOfInterest = filtered
	}
	if err := store.SaveFocus(ctx, state); err != nil {
		return err
	}
	if err := store.ClearCache(ctx, cmd.appOpts.ProjectID, cmd.appOpts.SessionID, ""); err != nil {
		return err
	}
	if args[0] == "add" {
		fmt.Fprintf(cmd.appOpts.Out(), "Added plan to interests: %s\n", name)
	} else {
		fmt.Fprintf(cmd.appOpts.Out(), "Removed plan from interests: %s\n", name)
	}
	return nil
}

// FocusShowCommand displays the current session anchor.
type FocusShowCommand struct {
	ReportFormat
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *FocusShowCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders current focus state.
func (cmd *FocusShowCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("focus show accepts no arguments")
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
	state, err := store.LoadFocus(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	initiativeName := displayTaskID(state.InitiativeID)
	if state.InitiativeID != "" {
		initiative, err := store.FindInitiativeByID(cmd.appOpts.Context(), cmd.appOpts.ProjectID, state.InitiativeID)
		if err != nil {
			return err
		}
		initiativeName = initiative.Name
	}
	if format != formatText {
		return writeReport(cmd.appOpts, format, report{command: "focus", records: []record{focusRecord(state, initiativeName)}})
	}
	printFocus(cmd.appOpts.Out(), state, initiativeName)
	return nil
}

func focusRecord(state task.FocusState, initiativeName string) record {
	if state.InitiativeID == "" {
		initiativeName = ""
	}
	stack := make([]string, 0, len(state.TaskStack))
	for _, entry := range state.TaskStack {
		stack = append(stack, entry.TaskID)
	}
	return record{
		{"project_id", state.ProjectID},
		{"session_id", state.SessionID},
		{"initiative_id", state.InitiativeID},
		{"initiative", initiativeName},
		{"task_id", state.FocusedTaskID},
		{"stack", stack},
		{"plans_of_interest", nonNilStrings(state.PlansOfInterest)},
	}
}

// FocusPlanCommand anchors a session to an initiative and its next ready task.
type FocusPlanCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *FocusPlanCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute focuses an initiative and pushes its next ready task.
func (cmd *FocusPlanCommand) Execute(args []string) error {
	return cmd.execute(args, false)
}

func (cmd *FocusPlanCommand) execute(args []string, independent bool) error {
	if len(args) != 1 {
		return fmt.Errorf("focus plan requires one initiative name\nACTION: Run 'jczl-flow help focus'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	initiative, err := store.FindInitiative(cmd.appOpts.Context(), cmd.appOpts.ProjectID, args[0])
	if err != nil {
		return err
	}
	state, err := store.LoadFocus(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	state.InitiativeID = initiative.ID
	state.FocusedTaskID = ""
	state.TaskStack = []task.FocusEntry{}
	ready, err := store.ReadyTasks(cmd.appOpts.Context(), cmd.appOpts.ProjectID, initiative.Name)
	if err != nil {
		return err
	}
	if len(ready) > 0 {
		state.FocusedTaskID = ready[0].ID
		state.TaskStack = []task.FocusEntry{{
			TaskID:       ready[0].ID,
			InitiativeID: initiative.ID,
		}}
	}
	if err := store.SaveFocus(cmd.appOpts.Context(), state); err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, task.Task{InitiativeName: initiative.Name}); err != nil {
		return err
	}
	if independent {
		fmt.Fprintf(cmd.appOpts.Out(), "Independent focus anchored to plan: %s\n", initiative.Name)
	} else {
		fmt.Fprintf(cmd.appOpts.Out(), "Focused initiative %s\n", initiative.Name)
	}
	if len(ready) > 0 {
		fmt.Fprintf(cmd.appOpts.Out(), "Next task %s: %s\n", shortID(ready[0].ID), ready[0].Description)
	}
	return nil
}

// FocusTaskCommand anchors a session to one task.
type FocusTaskCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *FocusTaskCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute focuses a task and preserves its initiative anchor.
func (cmd *FocusTaskCommand) Execute(args []string) error {
	return cmd.execute(args, false)
}

func (cmd *FocusTaskCommand) execute(args []string, independent bool) error {
	taskID, err := oneTaskID("focus task", args)
	if err != nil {
		return err
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	current, err := store.GetTask(cmd.appOpts.Context(), taskID)
	if err != nil {
		return err
	}
	state, err := store.LoadFocus(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	state.InitiativeID = current.InitiativeID
	state.FocusedTaskID = current.ID
	state.TaskStack = pushFocus(state.TaskStack, task.FocusEntry{
		TaskID:       current.ID,
		InitiativeID: current.InitiativeID,
	})
	if err := store.SaveFocus(cmd.appOpts.Context(), state); err != nil {
		return err
	}
	if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Focused task %s: %s\n", shortID(current.ID), current.Description)
	return nil
}

// FocusPopCommand returns to the previous task in the session stack.
type FocusPopCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *FocusPopCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute pops the current task anchor.
func (cmd *FocusPopCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("focus pop accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	state, err := store.LoadFocus(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID)
	if err != nil {
		return err
	}
	if len(state.TaskStack) == 0 {
		return fmt.Errorf("focus stack is empty\nACTION: Run 'jczl-flow focus task <uuid>' or 'jczl-flow focus plan <name>'.")
	}
	state.TaskStack = state.TaskStack[1:]
	state.FocusedTaskID = ""
	if len(state.TaskStack) > 0 {
		state.FocusedTaskID = state.TaskStack[0].TaskID
		state.InitiativeID = state.TaskStack[0].InitiativeID
	}
	if err := store.SaveFocus(cmd.appOpts.Context(), state); err != nil {
		return err
	}
	if state.FocusedTaskID == "" {
		if err := store.ClearCache(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID, ""); err != nil {
			return err
		}
	} else {
		current, err := store.GetTask(cmd.appOpts.Context(), state.FocusedTaskID)
		if err != nil {
			return err
		}
		if err := clearTaskCaches(store, cmd.appOpts, current); err != nil {
			return err
		}
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Focus popped; current task: %s\n", displayTaskID(state.FocusedTaskID))
	return nil
}

// FocusBackCommand exits the current independent session.
type FocusBackCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *FocusBackCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute deletes the current non-global session and falls back to global focus.
func (cmd *FocusBackCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("focus back accepts no arguments")
	}
	if cmd.appOpts.SessionID == "global" {
		return fmt.Errorf("cannot leave the global focus\nACTION: Set JACAZUL_SESSION_ID before using 'jczl-flow focus back'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.DeleteSession(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID); err != nil {
		return err
	}
	fmt.Fprintln(cmd.appOpts.Out(), "Switched back to global focus")
	return nil
}

// FocusClearCommand clears the current session anchor.
type FocusClearCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *FocusClearCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute clears the session focus without deleting the session.
func (cmd *FocusClearCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("focus clear accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	state := task.FocusState{
		ProjectID: cmd.appOpts.ProjectID,
		SessionID: cmd.appOpts.SessionID,
		TaskStack: []task.FocusEntry{},
	}
	if err := store.SaveFocus(cmd.appOpts.Context(), state); err != nil {
		return err
	}
	if err := store.ClearCache(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.appOpts.SessionID, ""); err != nil {
		return err
	}
	fmt.Fprintln(cmd.appOpts.Out(), "Focus cleared")
	return nil
}

func pushFocus(stack []task.FocusEntry, entry task.FocusEntry) []task.FocusEntry {
	filtered := make([]task.FocusEntry, 0, len(stack)+1)
	for _, current := range stack {
		if current.TaskID != entry.TaskID {
			filtered = append(filtered, current)
		}
	}
	return append([]task.FocusEntry{entry}, filtered...)
}

func printFocus(out io.Writer, state task.FocusState, initiativeName string) {
	fmt.Fprintln(out, "FOCUS")
	fmt.Fprintf(out, "Project: %s\n", state.ProjectID)
	fmt.Fprintf(out, "Session: %s\n", state.SessionID)
	fmt.Fprintf(out, "Initiative: %s\n", initiativeName)
	fmt.Fprintf(out, "Task: %s\n", displayTaskID(state.FocusedTaskID))
	fmt.Fprintln(out, "STACK:")
	for _, entry := range state.TaskStack {
		fmt.Fprintf(out, "- %s\n", shortID(entry.TaskID))
	}
}

func displayTaskID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "none"
	}
	return shortID(id)
}
