package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// PonderCommand renders the project-wide initiative dashboard.
type PonderCommand struct {
	ReportFormat
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
	format, err := cmd.resolve(cmd.appOpts)
	if err != nil {
		return err
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	if format != formatText {
		return reportPonder(store, cmd.appOpts, format, cmd.All, cmd.WithBacklog, cmd.Force)
	}
	return renderPonder(store, cmd.appOpts, cmd.All, cmd.WithBacklog, cmd.Table, cmd.Force)
}

// PlansCommand lists initiatives using the same dashboard model as ponder.
type PlansCommand struct {
	ReportFormat
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
	format, err := cmd.resolve(cmd.appOpts)
	if err != nil {
		return err
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	if format != formatText {
		return reportPlans(store, cmd.appOpts, format, cmd.All, cmd.Closed, cmd.WithBacklog, cmd.Force)
	}
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
	ReportFormat
	TagArguments
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
	format, err := cmd.resolve(cmd.appOpts)
	if err != nil {
		return err
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
	allTasks, err := store.ListTasks(cmd.appOpts.Context(), cmd.appOpts.ProjectID, initiative)
	if err != nil {
		return err
	}
	tasks := activeBucketTasks(filterTasksByTags(allTasks, cmd.TagFilter()))
	states := make(map[string]task.Task, len(allTasks))
	for _, current := range allTasks {
		states[current.ID] = current
	}
	if format != formatText {
		return reportTree(cmd.appOpts, store, format, tasks, states)
	}
	if len(tasks) == 0 {
		fmt.Fprintln(cmd.appOpts.Out(), "No tasks found.")
		return nil
	}
	for _, current := range tasks {
		tags := formatTaskTags(current.Tags)
		if tags != "" {
			tags += " "
		}
		fmt.Fprintf(cmd.appOpts.Out(), "[%s] %s %s%s\n", treeMarker(current, states), shortID(current.ID), tags, current.Description)
	}
	return nil
}

func treeMarker(current task.Task, states map[string]task.Task) string {
	switch {
	case current.Status == task.Completed:
		return "DONE"
	case current.Status == task.Active:
		return "ACTIVE"
	case !dependenciesReady(current, states):
		return "BLOCKED"
	}
	return "READY"
}

func reportTree(opts *config.AppOptions, store *sqlite.Store, format string, tasks []task.Task, states map[string]task.Task) error {
	records := make([]record, 0, len(tasks))
	for _, current := range tasks {
		built, err := taskRecord(opts.Context(), store, current)
		if err != nil {
			return err
		}
		records = append(records, append(built, field{"marker", treeMarker(current, states)}))
	}
	return writeReport(opts, format, report{command: "tree", records: records})
}

// reportPlans renders the plans records. It filters exactly as the text
// list does and carries the cache fact as meta.cached.
func reportPlans(store *sqlite.Store, opts *config.AppOptions, format string, all bool, closed bool, withBacklog bool, force bool) error {
	return reportDashboard(store, opts, format, "plans", plansCacheKey(all, closed, withBacklog), force, 5*time.Minute,
		func(summaries []task.InitiativeSummary, _ task.FocusState) []task.InitiativeSummary {
			return plansSummaries(summaries, all, closed)
		}, withBacklog)
}

// reportPonder renders the initiatives the ponder dashboard shows.
func reportPonder(store *sqlite.Store, opts *config.AppOptions, format string, all bool, withBacklog bool, force bool) error {
	return reportDashboard(store, opts, format, "ponder", ponderCacheKey(all, withBacklog, false), force, 10*time.Minute,
		func(summaries []task.InitiativeSummary, focus task.FocusState) []task.InitiativeSummary {
			visible, _ := visibleSummaries(summaries, focus, all)
			return visible
		}, withBacklog)
}

func reportDashboard(
	store *sqlite.Store,
	opts *config.AppOptions,
	format string,
	command string,
	cacheKey string,
	force bool,
	ttl time.Duration,
	selectSummaries func([]task.InitiativeSummary, task.FocusState) []task.InitiativeSummary,
	withBacklog bool,
) error {
	ctx := opts.Context()
	if !force {
		cached, found, err := loadCachedReport(ctx, store, opts, cacheKey)
		if err != nil {
			return err
		}
		if found {
			cached.command = command
			return writeReport(opts, format, cached)
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
	records := initiativeRecords(selectSummaries(summaries, focus))
	if err := writeReport(opts, format, report{command: command, records: records}); err != nil {
		return err
	}
	return storeCachedReport(ctx, store, opts, cacheKey, records, ttl)
}

func renderPlans(store *sqlite.Store, opts *config.AppOptions, all bool, closed bool, withBacklog bool, force bool) error {
	cacheKey := plansCacheKey(all, closed, withBacklog)
	ctx := opts.Context()
	if !force {
		_, found, err := store.GetCache(ctx, opts.ProjectID, opts.SessionID, cacheKey, time.Now().UTC())
		if err != nil {
			return err
		}
		if found {
			fmt.Fprintln(opts.Out(), "🐊 [cached] Plans unchanged. Use --force to refresh.")
			return nil
		}
	}
	summaries, err := store.ListInitiatives(ctx, opts.ProjectID, withBacklog, true)
	if err != nil {
		return err
	}
	output := renderPlanList(opts.ProjectID, summaries, all, closed)
	fmt.Fprint(opts.Out(), output)
	return store.SetCache(ctx, opts.ProjectID, opts.SessionID, cacheKey, output, time.Now().UTC().Add(5*time.Minute))
}

func plansCacheKey(all bool, closed bool, withBacklog bool) string {
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
	return cacheKey
}

// plansSummaries keeps the initiatives the plans list shows: open ones by
// default, completed ones with --closed, and both with --all.
func plansSummaries(summaries []task.InitiativeSummary, all bool, closed bool) []task.InitiativeSummary {
	shown := make([]task.InitiativeSummary, 0, len(summaries))
	for _, summary := range summaries {
		isClosed := summary.Initiative.Status == task.InitiativeCompleted
		if closed && !isClosed {
			continue
		}
		if !all && !closed && isClosed {
			continue
		}
		shown = append(shown, summary)
	}
	return shown
}

func renderPlanList(projectID string, summaries []task.InitiativeSummary, all bool, closed bool) string {
	var output strings.Builder
	fmt.Fprintf(&output, "PROJECT: %s\n", projectID)
	shown := 0
	for _, summary := range plansSummaries(summaries, all, closed) {
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
		if summary.Initiative.Metadata.Goal != "" {
			fmt.Fprintf(&output, "  Goal: %s\n", summary.Initiative.Metadata.Goal)
		}
	}
	if shown == 0 {
		output.WriteString("No initiatives found.\n")
	}
	return output.String()
}

func renderPonder(store *sqlite.Store, opts *config.AppOptions, all bool, withBacklog bool, table bool, force bool) error {
	cacheKey := ponderCacheKey(all, withBacklog, table)
	ctx := opts.Context()
	if !force {
		_, found, err := store.GetCache(ctx, opts.ProjectID, opts.SessionID, cacheKey, time.Now().UTC())
		if err != nil {
			return err
		}
		if found {
			fmt.Fprintln(opts.Out(), "🐊 [cached] Ponder unchanged. Use --force to refresh.")
			return nil
		}
	}
	summaries, tasks, err := store.ListInitiativesWithTasks(ctx, opts.ProjectID, withBacklog, true)
	if err != nil {
		return err
	}
	focus, err := store.LoadFocus(ctx, opts.ProjectID, opts.SessionID)
	if err != nil {
		return err
	}
	output, err := renderDashboard(ctx, store, opts, summaries, tasks, focus, all, table, true)
	if err != nil {
		return err
	}
	fmt.Fprint(opts.Out(), output)
	return store.SetCache(ctx, opts.ProjectID, opts.SessionID, cacheKey, output, time.Now().UTC().Add(10*time.Minute))
}

func ponderCacheKey(all bool, withBacklog bool, table bool) string {
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
	return cacheKey
}

// visibleSummaries keeps the focused initiative and the plans of interest
// when interests are set, and returns the focused initiative name.
func visibleSummaries(summaries []task.InitiativeSummary, focus task.FocusState, showAll bool) ([]task.InitiativeSummary, string) {
	focusedName := ""
	for _, summary := range summaries {
		if summary.Initiative.ID == focus.InitiativeID {
			focusedName = summary.Initiative.Name
			break
		}
	}
	visible := make([]task.InitiativeSummary, 0, len(summaries))
	for _, summary := range summaries {
		if !showAll && len(focus.PlansOfInterest) > 0 && summary.Initiative.Name != focusedName && !containsString(focus.PlansOfInterest, summary.Initiative.Name) {
			continue
		}
		visible = append(visible, summary)
	}
	return visible, focusedName
}

func groupTasksByInitiative(tasks []task.Task) map[string][]task.Task {
	grouped := make(map[string][]task.Task)
	for _, current := range tasks {
		grouped[current.InitiativeID] = append(grouped[current.InitiativeID], current)
	}
	return grouped
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
		return fmt.Errorf("%s requires one initiative name\nACTION: Run 'jczl-flow help %s'.", command, command)
	}
	store, err := openStore(opts)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.SetInitiativeStatus(opts.Context(), opts.ProjectID, args[0], state); err != nil {
		return err
	}
	if err := store.ClearCache(opts.Context(), opts.ProjectID, opts.SessionID, ""); err != nil {
		return err
	}
	fmt.Fprintf(opts.Out(), "Initiative %s: %s\n", args[0], command)
	return nil
}

func dependenciesReady(current task.Task, states map[string]task.Task) bool {
	for _, dependencyID := range current.Dependencies {
		dependency, ok := states[dependencyID]
		if !ok {
			return false
		}
		if dependency.Bucket != task.BucketActive {
			continue
		}
		if dependency.Status != task.Completed {
			return false
		}
	}
	return true
}
