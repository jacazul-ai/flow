package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jacazul-ai/jaflow/internal/config"
	"github.com/jacazul-ai/jaflow/internal/storage/sqlite"
	"github.com/jacazul-ai/jaflow/internal/task"
)

// PonderCommand renders the project-wide initiative dashboard.
type PonderCommand struct {
	All         bool `long:"all" description:"Include completed initiatives"`
	WithBacklog bool `long:"with-backlog" description:"Include backlog initiatives"`
	Table       bool `long:"table" description:"Render the tactical readout as a table"`
	Force       bool `long:"force" description:"Bypass the dashboard cache"`
	appOpts     *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *PonderCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders initiative counts for the current project.
func (cmd *PonderCommand) Execute(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("ponder accepts at most one project filter")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	return renderPonder(store, cmd.appOpts, cmd.All, cmd.WithBacklog, cmd.Table, cmd.Force)
}

// PlansCommand lists initiatives using the same dashboard model as ponder.
type PlansCommand struct {
	All         bool `long:"all" description:"Include completed initiatives"`
	Closed      bool `long:"closed" description:"Show completed initiatives only"`
	WithBacklog bool `long:"with-backlog" description:"Include backlog initiatives"`
	Force       bool `long:"force" description:"Bypass the dashboard cache"`
	appOpts     *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *PlansCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute lists initiative summaries.
func (cmd *PlansCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("plans accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	return renderPlans(store, cmd.appOpts, cmd.All, cmd.Closed, cmd.WithBacklog, cmd.Force)
}

// BacklogCommand hides an initiative from default dashboards.
type BacklogCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *BacklogCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute moves an initiative to backlog.
func (cmd *BacklogCommand) Execute(args []string) error {
	return setInitiativeState(cmd.appOpts, args, task.InitiativeBacklog, "backlog")
}

// ActivateCommand restores an initiative from backlog.
type ActivateCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *ActivateCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute activates an initiative.
func (cmd *ActivateCommand) Execute(args []string) error {
	return setInitiativeState(cmd.appOpts, args, task.InitiativeActive, "activate")
}

// TreeCommand renders dependency markers for an initiative.
type TreeCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *TreeCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute renders a compact dependency tree view.
func (cmd *TreeCommand) Execute(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("tree accepts at most one initiative name")
	}
	initiative := ""
	if len(args) == 1 {
		initiative = args[0]
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	tasks, err := store.ListTasks(context.Background(), cmd.appOpts.ProjectID, initiative)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}
	states := make(map[string]task.Status, len(tasks))
	for _, current := range tasks {
		states[current.ID] = current.Status
	}
	for _, current := range tasks {
		marker := "READY"
		if current.Status == task.Completed {
			marker = "DONE"
		} else if current.Status == task.Active {
			marker = "ACTIVE"
		} else if !dependenciesReady(current, states) {
			marker = "BLOCKED"
		}
		fmt.Printf("[%s] %s %s\n", marker, shortID(current.ID), current.Description)
	}
	return nil
}

func renderPlans(store *sqlite.Store, opts *config.AppOptions, all bool, closed bool, withBacklog bool, force bool) error {
	cacheKey := "plans"
	if all {
		cacheKey += "_all"
	}
	if closed {
		cacheKey += "_closed"
	}
	if withBacklog {
		cacheKey += "_backlog"
	}
	ctx := context.Background()
	if !force {
		_, found, err := store.GetCache(ctx, opts.ProjectID, opts.SessionID, cacheKey, time.Now().UTC())
		if err != nil {
			return err
		}
		if found {
			fmt.Println("🐊 [cached] Plans unchanged. Use --force to refresh.")
			return nil
		}
	}
	summaries, err := store.ListInitiatives(ctx, opts.ProjectID, withBacklog, true)
	if err != nil {
		return err
	}
	output := renderPlanList(opts.ProjectID, summaries, all, closed)
	fmt.Print(output)
	return store.SetCache(ctx, opts.ProjectID, opts.SessionID, cacheKey, output, time.Now().UTC().Add(5*time.Minute))
}

func renderPlanList(projectID string, summaries []task.InitiativeSummary, all bool, closed bool) string {
	var output strings.Builder
	fmt.Fprintf(&output, "PROJECT: %s\n", projectID)
	shown := 0
	for _, summary := range summaries {
		isClosed := summary.Initiative.Status == task.InitiativeCompleted
		if closed && !isClosed {
			continue
		}
		if !all && !closed && isClosed {
			continue
		}
		if shown == 0 {
			output.WriteString("INITIATIVES:\n")
		}
		shown++
		fmt.Fprintf(&output, "- [%s] %s [id:%s] pending:%d active:%d completed:%d blocked:%d\n",
			strings.ToUpper(string(summary.Initiative.Status)),
			summary.Initiative.Name,
			shortID(summary.Initiative.ID),
			summary.Pending,
			summary.Active,
			summary.Completed,
			summary.Blocked,
		)
	}
	if shown == 0 {
		output.WriteString("No initiatives found.\n")
	}
	return output.String()
}

func renderPonder(store *sqlite.Store, opts *config.AppOptions, all bool, withBacklog bool, table bool, force bool) error {
	cacheKey := "ponder"
	if all {
		cacheKey += "_all"
	}
	if withBacklog {
		cacheKey += "_backlog"
	}
	if table {
		cacheKey += "_table"
	}
	ctx := context.Background()
	if !force {
		_, found, err := store.GetCache(ctx, opts.ProjectID, opts.SessionID, cacheKey, time.Now().UTC())
		if err != nil {
			return err
		}
		if found {
			fmt.Println("🐊 [cached] Ponder unchanged. Use --force to refresh.")
			return nil
		}
	}
	summaries, err := store.ListInitiatives(ctx, opts.ProjectID, withBacklog, true)
	if err != nil {
		return err
	}
	focus, err := store.LoadFocus(ctx, opts.ProjectID, opts.SessionID)
	if err != nil {
		return err
	}
	output, err := renderDashboard(ctx, store, opts, summaries, focus, all, table)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return store.SetCache(ctx, opts.ProjectID, opts.SessionID, cacheKey, output, time.Now().UTC().Add(10*time.Minute))
}

func renderDashboard(ctx context.Context, store *sqlite.Store, opts *config.AppOptions, summaries []task.InitiativeSummary, focus task.FocusState, showAll bool, table bool) (string, error) {
	focusedName := ""
	for _, summary := range summaries {
		if summary.Initiative.ID == focus.InitiativeID {
			focusedName = summary.Initiative.Name
			break
		}
	}
	visibleSummaries := make([]task.InitiativeSummary, 0, len(summaries))
	visibleNames := make(map[string]struct{}, len(summaries))
	for _, summary := range summaries {
		if !showAll && len(focus.PlansOfInterest) > 0 && summary.Initiative.Name != focusedName && !containsString(focus.PlansOfInterest, summary.Initiative.Name) {
			continue
		}
		visibleSummaries = append(visibleSummaries, summary)
		visibleNames[summary.Initiative.Name] = struct{}{}
	}

	tasks, err := store.ListTasks(ctx, opts.ProjectID, "")
	if err != nil {
		return "", err
	}
	visibleTasks := make([]task.Task, 0, len(tasks))
	for _, current := range tasks {
		if _, ok := visibleNames[current.InitiativeName]; ok {
			visibleTasks = append(visibleTasks, current)
		}
	}

	var output strings.Builder
	output.WriteString(renderInitiatives(opts.ProjectID, visibleSummaries))
	appendSessionContext(&output, focus, focusedName)
	appendPulseSummary(&output, visibleTasks, summaries, focusedName)
	if err := appendTaskLandscape(ctx, store, &output, visibleSummaries); err != nil {
		return "", err
	}
	appendTacticalReadout(&output, visibleTasks, table)
	if err := appendRecentlyClosed(ctx, store, &output, summaries); err != nil {
		return "", err
	}
	return output.String(), nil
}

func renderInitiatives(projectID string, summaries []task.InitiativeSummary) string {
	var output strings.Builder
	fmt.Fprintf(&output, "PROJECT: %s\n", projectID)
	currentCount := 0
	for _, summary := range summaries {
		if summary.Initiative.Status != task.InitiativeCompleted {
			currentCount++
		}
	}
	if currentCount == 0 {
		output.WriteString("No initiatives found.\n")
		return output.String()
	}
	output.WriteString("INITIATIVES:\n")
	for _, summary := range summaries {
		if summary.Initiative.Status == task.InitiativeCompleted {
			continue
		}
		fmt.Fprintf(&output, "- [%s] %s pending:%d active:%d completed:%d blocked:%d\n",
			strings.ToUpper(string(summary.Initiative.Status)),
			summary.Initiative.Name,
			summary.Pending,
			summary.Active,
			summary.Completed,
			summary.Blocked,
		)
	}
	return output.String()
}

func appendSessionContext(output *strings.Builder, focus task.FocusState, focusedName string) {
	if focusedName == "" {
		focusedName = "None"
	}
	focusedTask := "None"
	if focus.FocusedTaskID != "" {
		focusedTask = shortID(focus.FocusedTaskID)
	}
	output.WriteString("SESSION CONTEXT:\n")
	fmt.Fprintf(output, "  Focus: %s | Task: %s\n", focusedName, focusedTask)
	if len(focus.TaskStack) > 0 {
		track := make([]string, 0, len(focus.TaskStack))
		for _, entry := range focus.TaskStack {
			track = append(track, shortID(entry.TaskID))
		}
		fmt.Fprintf(output, "  Track: %s\n", strings.Join(track, " -> "))
	}
	if len(focus.PlansOfInterest) > 0 {
		fmt.Fprintf(output, "  Interests: %s\n", strings.Join(focus.PlansOfInterest, ", "))
	}
	output.WriteString("\n")
}

func appendPulseSummary(output *strings.Builder, tasks []task.Task, summaries []task.InitiativeSummary, focusedName string) {
	today := time.Now().UTC().Format("2006-01-02")
	pending, active, overdue, completedToday := 0, 0, 0, 0
	plans := make(map[string]struct{})
	for _, current := range tasks {
		if current.Status == task.Pending || current.Status == task.Active {
			pending++
			plans[current.InitiativeName] = struct{}{}
		}
		if current.Status == task.Active {
			active++
		}
		if (current.Status == task.Pending || current.Status == task.Active) && current.DueAt != "" && current.DueAt < today {
			overdue++
		}
		if current.Status == task.Completed && strings.HasPrefix(current.CompletedAt, today) {
			completedToday++
		}
	}
	if focusedName == "" {
		focusedName = "None"
	}
	output.WriteString("[PULSE SUMMARY]\n")
	fmt.Fprintf(output, "  Focused: %s | Plans: %d | Done Today: %d\n", focusedName, len(plans), completedToday)
	fmt.Fprintf(output, "  Health | Pending: %d | Active: %d | Overdue: %d\n", pending, active, overdue)
	fmt.Fprintf(output, "  Registry | Initiatives: %d\n\n", len(summaries))
}

func appendTaskLandscape(ctx context.Context, store *sqlite.Store, output *strings.Builder, summaries []task.InitiativeSummary) error {
	output.WriteString("[TASK LANDSCAPE]\n")
	for _, summary := range summaries {
		tasks, err := store.ListTasks(ctx, summary.Initiative.ProjectID, summary.Initiative.Name)
		if err != nil {
			return err
		}
		ready, err := store.ReadyTasks(ctx, summary.Initiative.ProjectID, summary.Initiative.Name)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "  %s | Active: %d | Ready: %d | Total: %d\n", summary.Initiative.Name, summary.Active, len(ready), len(tasks))
	}
	output.WriteString("\n")
	return nil
}

func appendTacticalReadout(output *strings.Builder, tasks []task.Task, table bool) {
	readout := make([]task.Task, 0, len(tasks))
	for _, current := range tasks {
		if current.Status == task.Pending || current.Status == task.Active {
			readout = append(readout, current)
		}
	}
	sort.SliceStable(readout, func(i, j int) bool {
		if readout[i].Status != readout[j].Status {
			return readout[i].Status == task.Active
		}
		return readout[i].Urgency > readout[j].Urgency
	})
	output.WriteString("[TACTICAL READOUT]\n")
	if table {
		output.WriteString("| ST | UUID | MODE | PLAN | DESCRIPTION | URG |\n|---|---|---|---|---|---|\n")
		for _, current := range readout {
			fmt.Fprintf(output, "| %s | `%s` | %s | %s | %s | %.1f |\n", strings.ToUpper(string(current.Status)), shortID(current.ID), current.Mode.String(), current.InitiativeName, current.Description, current.Urgency)
		}
	} else {
		for _, current := range readout {
			fmt.Fprintf(output, "- [%s] %s | %s | %s | %s | [%.1f]\n", strings.ToUpper(string(current.Status)), shortID(current.ID), current.Mode.String(), current.InitiativeName, current.Description, current.Urgency)
		}
	}
	output.WriteString("\n")
}

func appendRecentlyClosed(ctx context.Context, store *sqlite.Store, output *strings.Builder, summaries []task.InitiativeSummary) error {
	closed := make([]task.InitiativeSummary, 0)
	for _, summary := range summaries {
		if summary.Initiative.Status == task.InitiativeCompleted {
			closed = append(closed, summary)
		}
	}
	if len(closed) == 0 {
		return nil
	}
	output.WriteString("[RECENTLY CLOSED]\n")
	for _, summary := range closed {
		tasks, err := store.ListTasks(ctx, summary.Initiative.ProjectID, summary.Initiative.Name)
		if err != nil {
			return err
		}
		outcome, err := latestOutcome(ctx, store, tasks)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "  ✓ %s completed:%d", summary.Initiative.Name, summary.Completed)
		if outcome != "" {
			fmt.Fprintf(output, " — %s", outcome)
		}
		output.WriteString("\n")
	}
	output.WriteString("\n")
	return nil
}

func latestOutcome(ctx context.Context, store *sqlite.Store, tasks []task.Task) (string, error) {
	latest := ""
	latestAt := ""
	for _, current := range tasks {
		annotations, err := store.ListAnnotations(ctx, current.ID)
		if err != nil {
			return "", err
		}
		for _, annotation := range annotations {
			if annotation.Kind == "OUTCOME" && annotation.CreatedAt >= latestAt {
				latest = annotation.Body
				latestAt = annotation.CreatedAt
			}
		}
	}
	return latest, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func setInitiativeState(opts *config.AppOptions, args []string, state task.InitiativeStatus, command string) error {
	if len(args) != 1 {
		return fmt.Errorf("%s requires one initiative name\nACTION: Run 'jaflow help %s'.", command, command)
	}
	store, err := openStore(opts)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.SetInitiativeStatus(context.Background(), opts.ProjectID, args[0], state); err != nil {
		return err
	}
	if err := store.ClearCache(context.Background(), opts.ProjectID, opts.SessionID, ""); err != nil {
		return err
	}
	fmt.Printf("Initiative %s: %s\n", args[0], command)
	return nil
}

func dependenciesReady(current task.Task, states map[string]task.Status) bool {
	for _, dependencyID := range current.Dependencies {
		if states[dependencyID] != task.Completed {
			return false
		}
	}
	return true
}
