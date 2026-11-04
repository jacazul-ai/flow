package cli

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// The dashboard follows the reference tw-flow ponder: it is agent context,
// so every section is bounded.
const (
	readoutLimit        = 15
	recentlyClosedLimit = 3
	closedOutcomeWidth  = 60
)

var (
	legacyModePattern = regexp.MustCompile(`\[([A-Z-]+)\]`)
	legacyModeToken   = regexp.MustCompile(`\[[A-Z-]+\]\s*`)
)

// dashboardView holds the task sets each dashboard section reads.
type dashboardView struct {
	focusedName string
	// visible are the active-bucket tasks of the initiatives the session
	// looks at: the focus and the plans of interest, or every plan.
	visible []task.Task
	// listed are the tasks of every listed (non-backlog) initiative, in any
	// bucket, for the project-wide pulse.
	listed []task.Task
	// all are every task of the project.
	all    []task.Task
	states map[string]task.Task
}

func newDashboardView(summaries []task.InitiativeSummary, visible []task.InitiativeSummary, tasks []task.Task, focusedName string) dashboardView {
	visibleIDs := make(map[string]struct{}, len(visible))
	for _, summary := range visible {
		visibleIDs[summary.Initiative.ID] = struct{}{}
	}
	listedIDs := make(map[string]struct{}, len(summaries))
	for _, summary := range summaries {
		listedIDs[summary.Initiative.ID] = struct{}{}
	}
	view := dashboardView{focusedName: focusedName, all: tasks, states: make(map[string]task.Task, len(tasks))}
	for _, current := range tasks {
		view.states[current.ID] = current
		if _, ok := listedIDs[current.InitiativeID]; ok {
			view.listed = append(view.listed, current)
		}
		if _, ok := visibleIDs[current.InitiativeID]; ok && current.Bucket == task.BucketActive {
			view.visible = append(view.visible, current)
		}
	}
	return view
}

// renderDashboard renders the ponder dashboard. The onboard briefing prints
// its own session context, so it passes withSessionContext false.
func renderDashboard(ctx context.Context, store *sqlite.Store, opts *config.AppOptions, summaries []task.InitiativeSummary, tasks []task.Task, focus task.FocusState, showAll bool, table bool, withSessionContext bool) (string, error) {
	visible, focusedName := visibleSummaries(summaries, focus, showAll)
	view := newDashboardView(summaries, visible, tasks, focusedName)

	var output strings.Builder
	fmt.Fprintf(&output, "PROJECT: %s\n", opts.ProjectID)
	if !hasOpenInitiative(summaries) {
		output.WriteString("No initiatives found.\n")
	}
	output.WriteString("\n")
	if withSessionContext {
		appendSessionContext(&output, focus, focusedName)
	}
	appendPulseSummary(&output, view)
	appendTaskLandscape(&output, visible, view)
	appendTacticalReadout(&output, view.visible, table)
	if err := appendRecentlyClosed(ctx, store, &output, summaries, tasks); err != nil {
		return "", err
	}
	return output.String(), nil
}

func hasOpenInitiative(summaries []task.InitiativeSummary) bool {
	for _, summary := range summaries {
		if summary.Initiative.Status != task.InitiativeCompleted {
			return true
		}
	}
	return false
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

func isOpen(current task.Task) bool {
	return current.Status == task.Pending || current.Status == task.Active
}

// appendPulseSummary renders the focused counters and the project-wide
// Global line with the archive and trash registry.
func appendPulseSummary(output *strings.Builder, view dashboardView) {
	today := time.Now().UTC().Format("2006-01-02")
	pending, active, overdue := 0, 0, 0
	plans := make(map[string]struct{})
	for _, current := range view.visible {
		if !isOpen(current) {
			continue
		}
		pending++
		plans[current.InitiativeID] = struct{}{}
		if current.Status == task.Active {
			active++
		}
		if current.DueAt != "" && current.DueAt < today {
			overdue++
		}
	}
	globalPending, globalActive := 0, 0
	for _, current := range view.listed {
		if !isOpen(current) {
			continue
		}
		globalPending++
		if current.Status == task.Active {
			globalActive++
		}
	}
	completedToday := 0
	archived := make(map[string]struct{})
	trashed := make(map[string]struct{})
	for _, current := range view.all {
		if current.Status == task.Completed && strings.HasPrefix(current.CompletedAt, today) {
			completedToday++
		}
		switch current.Bucket {
		case task.BucketArchive:
			archived[current.InitiativeID] = struct{}{}
		case task.BucketTrash:
			trashed[current.InitiativeID] = struct{}{}
		}
	}
	focusedName := view.focusedName
	if focusedName == "" {
		focusedName = "None"
	}
	output.WriteString("[PULSE SUMMARY]\n")
	fmt.Fprintf(output, "  Focused: %s | Plans: %d | Done Today: %d\n", focusedName, len(plans), completedToday)
	fmt.Fprintf(output, "  Health | Pending: %d | Active: %d | Overdue: %d\n", pending, active, overdue)
	fmt.Fprintf(output, "  Global | Pending: %d | Active: %d | Registry: %d Arch / %d Trash\n\n", globalPending, globalActive, len(archived), len(trashed))
}

// appendTaskLandscape lists only the plans with pending work: plans with
// nothing left to do are found through the closed-plans views.
func appendTaskLandscape(output *strings.Builder, visible []task.InitiativeSummary, view dashboardView) {
	open := make(map[string][]task.Task)
	for _, current := range view.visible {
		if isOpen(current) {
			open[current.InitiativeID] = append(open[current.InitiativeID], current)
		}
	}
	output.WriteString("[TASK LANDSCAPE]\n")
	for _, summary := range visible {
		tasks := open[summary.Initiative.ID]
		if len(tasks) == 0 {
			continue
		}
		active, ready := 0, 0
		for _, current := range tasks {
			if current.Status == task.Active {
				active++
				continue
			}
			if dependenciesReady(current, view.states) {
				ready++
			}
		}
		fmt.Fprintf(output, "  %s | Active: %d | Ready: %d | Total: %d\n", summary.Initiative.Name, active, ready, len(tasks))
		if goal := summary.Initiative.Metadata.Goal; goal != "" {
			fmt.Fprintf(output, "    Goal: %s\n", goal)
		}
	}
	output.WriteString("\n")
}

// appendTacticalReadout lists the most urgent open tasks, active first.
func appendTacticalReadout(output *strings.Builder, tasks []task.Task, table bool) {
	readout := make([]task.Task, 0, len(tasks))
	for _, current := range tasks {
		if isOpen(current) {
			readout = append(readout, current)
		}
	}
	sort.SliceStable(readout, func(i, j int) bool {
		if readout[i].Status != readout[j].Status {
			return readout[i].Status == task.Active
		}
		return readout[i].UrgencyScore > readout[j].UrgencyScore
	})
	if len(readout) > readoutLimit {
		readout = readout[:readoutLimit]
	}
	output.WriteString("[TACTICAL READOUT]\n")
	if table {
		output.WriteString("| ST | UUID | MODE | PLAN | TAGS | DESCRIPTION | URG |\n|---|---|---|---|---|---|---|\n")
	}
	for _, current := range readout {
		mode, description := displayModeAndDescription(current)
		format := "- [%s] %s | %s | %s | %s | %s | [%.1f]\n"
		if table {
			format = "| %s | `%s` | %s | %s | %s | %s | %.1f |\n"
		}
		fmt.Fprintf(output, format, strings.ToUpper(string(current.Status)), shortID(current.ID), mode, current.InitiativeName, formatTaskTags(current.Tags), description, current.UrgencyScore)
	}
	output.WriteString("\n")
}

// displayModeAndDescription shows the stored mode, else a legacy [XXX]
// prefix of the description, and keeps such prefixes out of the description.
func displayModeAndDescription(current task.Task) (string, string) {
	description := strings.TrimSpace(legacyModeToken.ReplaceAllString(current.Description, ""))
	if current.Mode != task.ModeUnspecified {
		return current.Mode.String(), description
	}
	if match := legacyModePattern.FindStringSubmatch(current.Description); match != nil {
		return match[1], description
	}
	return "--------", description
}

// closedPlan is an initiative with completed work and nothing left open.
type closedPlan struct {
	summary  task.InitiativeSummary
	closedAt string
	tasks    []task.Task
}

// appendRecentlyClosed lists the most recently closed plans, newest first.
func appendRecentlyClosed(ctx context.Context, store *sqlite.Store, output *strings.Builder, summaries []task.InitiativeSummary, tasks []task.Task) error {
	closed := recentlyClosedPlans(summaries, tasks)
	if len(closed) == 0 {
		return nil
	}
	taskIDs := make([]string, 0)
	for _, plan := range closed {
		for _, current := range plan.tasks {
			taskIDs = append(taskIDs, current.ID)
		}
	}
	annotations, err := store.ListAnnotationsForTasks(ctx, taskIDs)
	if err != nil {
		return err
	}
	output.WriteString("[RECENTLY CLOSED]\n")
	for _, plan := range closed {
		fmt.Fprintf(output, "  ✓ %s [%s]", plan.summary.Initiative.Name, closedDate(plan.closedAt))
		if outcome := latestOutcome(plan.tasks, annotations); outcome != "" {
			fmt.Fprintf(output, " — %s", cutRunes(outcome, closedOutcomeWidth))
		}
		output.WriteString("\n")
	}
	output.WriteString("\n")
	return nil
}

func recentlyClosedPlans(summaries []task.InitiativeSummary, tasks []task.Task) []closedPlan {
	byInitiative := make(map[string][]task.Task)
	for _, current := range tasks {
		if current.Bucket == task.BucketActive {
			byInitiative[current.InitiativeID] = append(byInitiative[current.InitiativeID], current)
		}
	}
	var closed []closedPlan
	for _, summary := range summaries {
		plan := closedPlan{summary: summary, tasks: byInitiative[summary.Initiative.ID]}
		open := false
		for _, current := range plan.tasks {
			if isOpen(current) {
				open = true
			}
			if current.Status == task.Completed && current.CompletedAt > plan.closedAt {
				plan.closedAt = current.CompletedAt
			}
		}
		if !open && plan.closedAt != "" {
			closed = append(closed, plan)
		}
	}
	sort.SliceStable(closed, func(i, j int) bool {
		return closed[i].closedAt > closed[j].closedAt
	})
	if len(closed) > recentlyClosedLimit {
		closed = closed[:recentlyClosedLimit]
	}
	return closed
}

func closedDate(value string) string {
	if len(value) < 10 {
		return "?"
	}
	return value[:10]
}

func cutRunes(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width])
}

func latestOutcome(tasks []task.Task, annotationsByTask map[string][]task.Annotation) string {
	latest := ""
	latestAt := ""
	for _, current := range tasks {
		for _, annotation := range annotationsByTask[current.ID] {
			if annotation.Kind == "OUTCOME" && annotation.CreatedAt >= latestAt {
				latest = annotation.Body
				latestAt = annotation.CreatedAt
			}
		}
	}
	return latest
}
