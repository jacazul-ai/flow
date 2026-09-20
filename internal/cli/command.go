package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
)

// HelpCommand renders agent-facing workflow guidance.
type HelpCommand struct {
	appOpts *config.AppOptions
}

// NewHelpCommand creates a help command backed by the command registry.
func NewHelpCommand() *HelpCommand {
	return &HelpCommand{}
}

// SetAppOptions supplies the output streams to the command.
func (cmd *HelpCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute prints root help or the operational brief for one command.
func (cmd *HelpCommand) Execute(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("help accepts at most one command name")
	}

	command := ""
	if len(args) == 1 {
		command = args[0]
	}

	registry := NewCommandRegistry()
	if command != "" && command != "help" {
		entry, ok := registry.Find(command)
		if !ok {
			return fmt.Errorf("unknown help topic %q; use 'jczl-flow help' to list commands", command)
		}
		writeCommandHelp(cmd.appOpts.Out(), entry)
		return nil
	}

	writeRootHelp(cmd.appOpts.Out())
	return nil
}

type helpEntry struct {
	name          string
	group         string
	canonical     string
	hidden        bool
	summary       string
	usage         string
	role          string
	preconditions []string
	effects       []string
	examples      []string
	next          string
}

const (
	groupStartAndOrganize   = "start and organize work"
	groupExamineState       = "examine workflow state"
	groupWorkCurrentTask    = "work on the current task"
	groupChangeReprioritize = "change and reprioritize work"
	groupPreserveContext    = "preserve and maintain context"
	groupMaintainDerived    = "maintain derived workflow state"
	groupPrepareIntegrate   = "prepare and integrate changes"
	groupMigrateLegacy      = "migrate legacy state"
)

var helpEntries = []helpEntry{
	{
		name:      "help",
		group:     groupExamineState,
		canonical: "help",
		summary:   "Show the agent workflow briefing",
		usage:     "jczl-flow help [command]",
		role:      "Use this before acting when the command contract or next step is unclear.",
		effects: []string{
			"Reads command metadata and renders guidance; it does not change workflow state.",
		},
		examples: []string{"jczl-flow help", "jczl-flow help plan"},
		next:     "Run 'jczl-flow status' to inspect the current project state.",
	},
	{
		name:      "onboard",
		group:     groupExamineState,
		canonical: "onboard",
		summary:   "Render a one-shot agent context briefing",
		usage:     "jczl-flow onboard",
		role:      "Use this once at agent bootstrap to load handoff, focus, context, and the current workflow view in a deterministic order.",
		effects: []string{
			"Renders the pending session handoff before focus and workflow context.",
			"Uses focused status when an anchor exists and the project dashboard otherwise.",
			"Acknowledges an unacknowledged handoff only after the complete briefing renders successfully.",
		},
		examples: []string{"jczl-flow onboard"},
		next:     "Continue with the focused task, or use the lower-level focus, status, ponder, and session commands for diagnostics.",
	},
	{
		name:      "plan",
		group:     groupStartAndOrganize,
		canonical: "plan",
		summary:   "Create an initiative and chained tasks",
		usage:     "jczl-flow plan <initiative> <task> [<task>...]",
		role:      "Use this to create a first-class initiative and its ordered work chain.",
		preconditions: []string{
			"A project identity must resolve from --project-id or PROJECT_ID.",
			"Provide an initiative name and at least one non-empty task description.",
		},
		effects: []string{
			"Creates the initiative in the project's SQLite database when absent.",
			"Creates one task per description; each task after the first depends on the previous task.",
			"Prints short UUIDs for the created tasks; full UUIDs remain the persisted identity.",
		},
		examples: []string{
			"jczl-flow plan parity 'Define schema' 'Implement store' 'Add tests'",
			"jczl-flow plan parity --database-path /tmp/parity.sqlite 'Define schema'",
		},
		next: "Run 'jczl-flow status <initiative>' to inspect pending work, then execute the first ready task.",
	},
	{
		name:      "organize",
		group:     groupStartAndOrganize,
		canonical: "organize",
		summary:   "Reorder pending tasks within one initiative",
		usage:     "jczl-flow organize <order|first|after|block> <initiative> ...",
		role:      "Use this to change presentation order without changing lifecycle state or dependency readiness.",
		preconditions: []string{
			"Every task reference must resolve uniquely in the selected project and initiative.",
			"Only pending tasks may be moved or used as relative anchors.",
		},
		effects: []string{
			"Persists an initiative-scoped task position and invalidates derived output cache.",
			"Keeps completed and active tasks immutable; blocked pending tasks remain blocked.",
			"Does not add dependencies, change task status, or change ready-task selection.",
		},
		examples: []string{
			"jczl-flow organize order parity 9a1b2c3d 8e7f6a5b",
			"jczl-flow organize first parity 9a1b2c3d",
			"jczl-flow organize after parity 9a1b2c3d 8e7f6a5b",
			"jczl-flow organize block parity 9a1b2c3d 8e7f6a5b --after 7d6c5b4a",
		},
		next: "Run 'jczl-flow status <initiative> --force' to verify the new order.",
	},
	{
		name:      "status",
		group:     groupExamineState,
		canonical: "status",
		summary:   "Show pending tasks for the current project",
		usage:     "jczl-flow status [initiative]",
		role:      "Use this as the hands-on view before switching focus or starting work.",
		preconditions: []string{
			"The project database is selected from --database-path or PROJECT_ID.",
		},
		effects: []string{
			"Reads only the selected project's task state; another project's database is not queried.",
			"An empty database returns a quiet 'No tasks found.' result.",
			"Pending tasks are printed with short UUIDs and descriptions.",
		},
		examples: []string{"jczl-flow status", "jczl-flow status parity"},
		next:     "Choose the first ready task in the initiative chain; do not skip a blocking dependency.",
	},
	{
		name:      "history",
		group:     groupExamineState,
		canonical: "history",
		summary:   "Show task or initiative history",
		usage:     "jczl-flow history task <uuid> | initiative|ini|plan <reference>",
		role:      "Use this to inspect immutable workflow events without changing state.",
		effects:   []string{"Reads task or initiative history in chronological order using an explicit scope."},
		examples:  []string{"jczl-flow history task 57c3fc80", "jczl-flow history initiative parity"},
		next:      "Use 'jczl-flow context <uuid>' for structured annotations and inherited context.",
	},
	{
		name:      "active",
		group:     groupExamineState,
		canonical: "active",
		summary:   "List active tasks",
		usage:     "jczl-flow active [initiative]",
		role:      "Use this to inspect tasks currently being executed in the project or one initiative.",
		effects:   []string{"Prints only tasks with active status and their direct or inherited ticket context."},
		examples:  []string{"jczl-flow active", "jczl-flow active parity"},
		next:      "Use 'jczl-flow status' for the full workflow view.",
	},
	{
		name:      "blocked",
		group:     groupExamineState,
		canonical: "blocked",
		summary:   "List blocked tasks",
		usage:     "jczl-flow blocked [initiative]",
		role:      "Use this to inspect pending tasks whose dependencies are unfinished.",
		effects:   []string{"Prints pending tasks that cannot start because a dependency is not completed."},
		examples:  []string{"jczl-flow blocked", "jczl-flow blocked parity"},
		next:      "Complete the blocking dependency before executing the task.",
	},
	{
		name:      "overdue",
		group:     groupExamineState,
		canonical: "overdue",
		summary:   "List overdue tasks",
		usage:     "jczl-flow overdue [initiative]",
		role:      "Use this to inspect pending tasks with a due date before today.",
		effects:   []string{"Prints pending tasks whose normalized due date has elapsed."},
		examples:  []string{"jczl-flow overdue", "jczl-flow overdue parity"},
		next:      "Review the task, update its plan, or execute it when dependencies are ready.",
	},
	{
		name:      "execute",
		group:     groupWorkCurrentTask,
		canonical: "execute",
		summary:   "Start a ready task",
		usage:     "jczl-flow execute <task-uuid>",
		role:      "Use this to begin work on the next task whose dependencies are complete.",
		preconditions: []string{
			"The task must exist in the selected project database.",
			"Every dependency must be completed; blocked tasks fail with an ACTION.",
		},
		effects: []string{
			"Moves the task from pending to active and records its start time.",
			"Does not silently switch to another initiative.",
		},
		examples: []string{"jczl-flow execute 57c3fc80"},
		next:     "Do the work, then run 'jczl-flow outcome <uuid> <message>' before done.",
	},
	{
		name:      "outcome",
		group:     groupWorkCurrentTask,
		canonical: "outcome",
		summary:   "Record the result required for completion",
		usage:     "jczl-flow outcome <task-uuid> <message...>",
		role:      "Use this to persist the result and handoff context before closing a task.",
		effects: []string{
			"Stores the outcome on the task and adds an OUTCOME annotation.",
			"Does not complete the task by itself.",
		},
		examples: []string{"jczl-flow outcome 57c3fc80 'Schema contract implemented'"},
		next:     "Run 'jczl-flow done <uuid>' after the outcome is recorded.",
	},
	{
		name:      "note",
		group:     groupPreserveContext,
		canonical: "note",
		summary:   "Add or delete structured task context",
		usage:     "jczl-flow note <task-uuid> <type> <message...>",
		role:      "Use this to persist decisions, research, outcomes, handoffs, and other task context.",
		preconditions: []string{
			"The task must exist in the selected project database.",
			"Use a supported semantic type or 'delete' with a timestamp.",
		},
		effects: []string{
			"Stores an uppercase annotation kind and message with a creation timestamp.",
			"Notes remain allowed on completed tasks; delete removes one timestamped annotation.",
		},
		examples: []string{
			"jczl-flow note 57c3fc80 decision 'Use the native store'",
			"jczl-flow note 57c3fc80 delete 2026-08-29T21:36:32.123Z",
		},
		next: "Run 'jczl-flow notes <uuid>' to inspect annotations or 'jczl-flow context <uuid>' for inherited context.",
	},
	{
		name:      "notes",
		group:     groupPreserveContext,
		canonical: "notes",
		summary:   "List task annotations",
		usage:     "jczl-flow notes <task-uuid>",
		role:      "Use this to inspect the durable annotations attached to one task.",
		effects:   []string{"Prints annotation timestamps, semantic kinds, and messages."},
		examples:  []string{"jczl-flow notes 57c3fc80"},
		next:      "Use a listed timestamp with 'jczl-flow note <uuid> delete <timestamp>' when removal is required.",
	},
	{
		name:      "context",
		group:     groupPreserveContext,
		canonical: "context",
		summary:   "Show direct and inherited task context",
		usage:     "jczl-flow context <task-uuid>",
		role:      "Use this to inspect annotations on a task and relevant dependency ancestors.",
		effects: []string{
			"Shows direct annotations and recursively inherited context in dependency-first order.",
			"Dependency cycles are bounded and cannot recurse indefinitely.",
		},
		examples: []string{"jczl-flow context 57c3fc80"},
		next:     "Use 'jczl-flow status <initiative>' for the focused workflow view with inherited context.",
	},
	{
		name:      "ticket",
		group:     groupPreserveContext,
		canonical: "ticket",
		summary:   "Link a task to an external ticket",
		usage:     "jczl-flow ticket <task-uuid> <ticket>",
		role:      "Use this to persist an external issue or ticket reference for commit and status awareness.",
		preconditions: []string{
			"The task must exist and must not be completed.",
			"Provide a non-empty external ticket reference.",
		},
		effects: []string{
			"Stores the direct ticket on the task and clears affected status and dashboard cache entries.",
			"Dependent tasks resolve the first available ticket recursively when no direct ticket exists.",
		},
		examples: []string{"jczl-flow ticket 57c3fc80 '#JAF-123'"},
		next:     "Run 'jczl-flow status <initiative>' to verify direct or inherited ticket awareness.",
	},
	{
		name:      "handoff",
		group:     groupWorkCurrentTask,
		canonical: "handoff",
		summary:   "Start a task with handoff context",
		usage:     "jczl-flow handoff <task-uuid> <message...>",
		role:      "Use this to transfer execution context and begin the next ready task.",
		preconditions: []string{
			"The target task must exist and its dependencies must be completed.",
			"The target task must not already be completed.",
		},
		effects: []string{
			"Starts the target task when pending and records a HANDOFF annotation.",
			"Preserves the dependency chain and clears affected derived views.",
		},
		examples: []string{"jczl-flow handoff 57c3fc80 'Start implementation with the validated design'"},
		next:     "Run 'jczl-flow notes <uuid>' or 'jczl-flow context <uuid>' to verify the handoff.",
	},
	{
		name:      "done",
		group:     groupWorkCurrentTask,
		canonical: "done",
		summary:   "Complete a task and expose ready work",
		usage:     "jczl-flow done <task-uuid>",
		role:      "Use this only after the task outcome is recorded.",
		preconditions: []string{
			"The task must not already be completed.",
			"An OUTCOME record is mandatory.",
		},
		effects: []string{
			"Marks the task completed and reports tasks newly unblocked in the same initiative.",
			"Preserves the dependency chain for the next focus switch.",
		},
		examples: []string{"jczl-flow done 57c3fc80"},
		next:     "Switch focus to the reported ready task, then run 'jczl-flow execute <uuid>'.",
	},
	{
		name:      "reopen",
		group:     groupWorkCurrentTask,
		canonical: "reopen",
		summary:   "Return a completed task to pending",
		usage:     "jczl-flow reopen <task-uuid>",
		role:      "Use this when more work is required on a completed task.",
		effects: []string{
			"Moves the task back to pending and clears completion disposition.",
			"Keeps the recorded outcome as historical context.",
		},
		examples: []string{"jczl-flow reopen 57c3fc80"},
		next:     "Run 'jczl-flow execute <uuid>' after its dependencies are ready.",
	},
	{
		name:      "discard",
		group:     groupWorkCurrentTask,
		canonical: "discard",
		summary:   "Discard a task with an audit outcome",
		usage:     "jczl-flow discard <task-uuid>",
		role:      "Use this instead of manually deleting or marking a task discarded.",
		effects: []string{
			"Completes the task with a discarded disposition.",
			"Adds an auditable OUTCOME record explaining the discard.",
		},
		examples: []string{"jczl-flow discard 57c3fc80"},
		next:     "Run 'jczl-flow status' to inspect the remaining initiative chain.",
	},
	{
		name:      "focus",
		group:     groupWorkCurrentTask,
		canonical: "focus",
		summary:   "Switch the project and session focus",
		usage:     "jczl-flow focus [<initiative>|show|plan|ini|task|pop|clear|back|ind|interest] [value]",
		role:      "Use this to move or inspect the agent anchor without losing the initiative chain.",
		preconditions: []string{
			"Focus is scoped to the selected PROJECT_ID and session ID.",
			"focus task accepts a full UUID or an unambiguous short UUID.",
		},
		effects: []string{
			"focus plan anchors the initiative and its next ready task.",
			"focus task pushes a task onto the session stack; focus pop returns to the previous task.",
			"focus clear removes the anchor but preserves the session record.",
		},
		examples: []string{
			"jczl-flow focus show",
			"jczl-flow focus plan parity",
			"jczl-flow focus task 57c3fc80",
			"jczl-flow focus pop",
		},
		next: "Run 'jczl-flow execute <uuid>' only after the focused task is ready; use 'jczl-flow focus ind' for an isolated session.",
	},
	{
		name:      "session",
		group:     groupPreserveContext,
		canonical: "session",
		summary:   "Manage native project sessions",
		usage:     "jczl-flow session <list|show|resume|ack|dump|purge>",
		role:      "Use this to inspect anchors, resume handoffs, and manage persisted session state.",
		preconditions: []string{
			"Session state is scoped to the selected project and session ID.",
			"session purge requires --confirm before deleting orphan sessions.",
		},
		effects: []string{
			"session list shows persisted sessions, anchors, age, and activity status.",
			"session resume and session ack expose the handoff lifecycle without replaying acknowledged notes.",
			"session dump creates a resumable handoff; session purge removes non-current sessions older than eight hours.",
		},
		examples: []string{
			"jczl-flow session list",
			"jczl-flow session dump",
			"jczl-flow session purge --confirm",
		},
		next: "Use 'jczl-flow focus task <uuid>' to switch the current session anchor.",
	},
	{
		name:      "ponder",
		group:     groupExamineState,
		canonical: "ponder",
		summary:   "Render the project initiative dashboard",
		usage:     "jczl-flow ponder [--all] [--with-backlog] [--force]",
		role:      "Use this as the horizon view for initiative health and blocked work.",
		effects: []string{
			"Shows pending, active, completed, and blocked counts per initiative.",
			"Uses a project/session-scoped cache unless --force is supplied.",
		},
		examples: []string{"jczl-flow ponder", "jczl-flow ponder --with-backlog --force"},
		next:     "Use 'jczl-flow status' for the focused initiative and 'jczl-flow focus' to switch.",
	},
	{
		name:      "plans",
		group:     groupExamineState,
		canonical: "plans",
		summary:   "List initiative summaries",
		usage:     "jczl-flow plans [--all|--closed] [--with-backlog]",
		role:      "Use this to compare initiative lifecycle and work counts.",
		effects:   []string{"Backlog initiatives are hidden unless --with-backlog is supplied; --closed shows completed initiatives only."},
		examples:  []string{"jczl-flow plans", "jczl-flow plans --all", "jczl-flow plans --closed"},
		next:      "Choose an initiative and run 'jczl-flow focus plan <name>'.",
	},
	{
		name:      "backlog",
		group:     groupStartAndOrganize,
		canonical: "backlog",
		summary:   "Hide an initiative from default dashboards",
		usage:     "jczl-flow backlog <initiative>",
		role:      "Use this when an initiative is intentionally paused but must remain recoverable.",
		effects:   []string{"Marks the initiative as backlog and clears derived dashboard cache."},
		examples:  []string{"jczl-flow backlog old-plan"},
		next:      "Use 'jczl-flow activate <initiative>' when work resumes.",
	},
	{
		name:      "activate",
		group:     groupStartAndOrganize,
		canonical: "activate",
		summary:   "Restore a backlog initiative",
		usage:     "jczl-flow activate <initiative>",
		role:      "Use this to return a paused initiative to normal dashboards.",
		effects:   []string{"Marks the initiative active and clears derived dashboard cache."},
		examples:  []string{"jczl-flow activate old-plan"},
		next:      "Run 'jczl-flow focus plan <initiative>' to resume work.",
	},
	{
		name:      "tree",
		group:     groupExamineState,
		canonical: "tree",
		summary:   "Show task dependency markers",
		usage:     "jczl-flow tree [initiative]",
		role:      "Use this to inspect ready, blocked, active, and completed tasks together.",
		effects:   []string{"Reads dependency edges without changing task state."},
		examples:  []string{"jczl-flow tree parity"},
		next:      "Start the first READY task; blocked tasks need their dependency completed.",
	},
	{
		name:      "commit",
		group:     groupPrepareIntegrate,
		canonical: "commit",
		summary:   "Draft a conventional commit from focus",
		usage:     "jczl-flow commit [--fix]",
		role:      "Use this after validation to derive a commit title from the focused task.",
		preconditions: []string{
			"A task must be focused in the current project session.",
			"The draft does not execute Git or stage files.",
		},
		effects: []string{
			"Maps task mode to a conventional commit type.",
			"Carries an inherited ticket as Refs unless --fix is explicitly supplied.",
		},
		examples: []string{"jczl-flow commit", "jczl-flow commit --fix"},
		next:     "Review the draft, stage only task-relevant files, and use git commit -F with approval.",
	},
	{
		name:      "roadmap",
		group:     groupStartAndOrganize,
		canonical: "roadmap",
		summary:   "Manage the project roadmap ledger",
		usage:     "jczl-flow roadmap <show|init|add|ship>",
		role:      "Use this to keep strategic initiative phases separate from operational task chains.",
		preconditions: []string{
			"roadmap init is allowed only when no roadmap ledger exists for the project.",
			"roadmap add requires --phase and --description.",
		},
		effects: []string{
			"roadmap init projects current initiatives into strategic phases.",
			"roadmap ship marks one ledger entry shipped and preserves its history.",
			"Duplicate initialization fails with an ACTION instead of creating a second ledger.",
		},
		examples: []string{
			"jczl-flow roadmap show",
			"jczl-flow roadmap init",
			"jczl-flow roadmap add --phase next --description 'Define release plan'",
			"jczl-flow roadmap ship roadmap-123",
		},
		next: "Use 'jczl-flow roadmap show' before changing an existing phase.",
	},
	{
		name:      "initiative",
		group:     groupStartAndOrganize,
		canonical: "plan",
		hidden:    true,
		summary:   "Alias for creating an initiative",
		usage:     "jczl-flow initiative <initiative> <task> [<task>...]",
		role:      "Use this compatibility alias for 'jczl-flow plan'.",
		effects:   []string{"Creates the same ordered initiative task chain as 'jczl-flow plan'."},
		examples:  []string{"jczl-flow initiative parity 'Define contract'"},
		next:      "Run 'jczl-flow status <initiative>' to inspect the created work.",
	},
	{
		name:      "ini",
		group:     groupStartAndOrganize,
		canonical: "plan",
		hidden:    true,
		summary:   "Short alias for creating an initiative",
		usage:     "jczl-flow ini <initiative> <task> [<task>...]",
		role:      "Use this compatibility alias for 'jczl-flow plan'.",
		effects:   []string{"Creates the same ordered initiative task chain as 'jczl-flow plan'."},
		examples:  []string{"jczl-flow ini parity 'Define contract'"},
		next:      "Run 'jczl-flow status <initiative>' to inspect the created work.",
	},
	{
		name:      "inis",
		group:     groupExamineState,
		canonical: "plans",
		hidden:    true,
		summary:   "Alias for listing initiatives",
		usage:     "jczl-flow inis [--all] [--closed] [--with-backlog]",
		role:      "Use this compatibility alias for 'jczl-flow plans'.",
		effects:   []string{"Lists the same initiative summaries as 'jczl-flow plans'."},
		examples:  []string{"jczl-flow inis --all"},
		next:      "Focus the initiative with 'jczl-flow focus plan <initiative>'.",
	},
	{
		name:      "initiatives",
		group:     groupExamineState,
		canonical: "plans",
		hidden:    true,
		summary:   "Alias for listing initiatives",
		usage:     "jczl-flow initiatives [--all] [--closed] [--with-backlog]",
		role:      "Use this compatibility alias for 'jczl-flow plans'.",
		effects:   []string{"Lists the same initiative summaries as 'jczl-flow plans'."},
		examples:  []string{"jczl-flow initiatives --all"},
		next:      "Focus the initiative with 'jczl-flow focus plan <initiative>'.",
	},
	{
		name:      "next",
		group:     groupExamineState,
		canonical: "next",
		summary:   "List ready tasks",
		usage:     "jczl-flow next [initiative]",
		role:      "Use this to find executable work without selecting blocked pending tasks.",
		effects: []string{
			"Lists pending tasks whose dependencies are completed.",
			"Reports no ready work without changing focus or task state.",
		},
		examples: []string{"jczl-flow next parity"},
		next:     "Run 'jczl-flow execute <uuid>' for a ready task.",
	},
	{
		name:      "amend",
		group:     groupChangeReprioritize,
		canonical: "amend",
		summary:   "Update task metadata",
		usage:     "jczl-flow amend <task-uuid> [description=\"...\"] [ticket=\"...\"]",
		role:      "Use this to correct metadata without reopening completed work.",
		effects:   []string{"Updates task description and/or external ticket metadata."},
		examples:  []string{"jczl-flow amend 57c3fc80 description=\"Updated contract\""},
		next:      "Run 'jczl-flow status <initiative>' to verify the metadata.",
	},
	{
		name:      "rename",
		group:     groupStartAndOrganize,
		canonical: "rename",
		summary:   "Rename an initiative",
		usage:     "jczl-flow rename <old-name> <new-name>",
		role:      "Use this to rename an initiative while preserving its task identities.",
		effects:   []string{"Updates the initiative name and keeps task, focus, and ticket relationships intact."},
		examples:  []string{"jczl-flow rename old-plan new-plan"},
		next:      "Run 'jczl-flow status <new-name>' to verify the rename.",
	},
	{
		name:      "urgent",
		group:     groupChangeReprioritize,
		canonical: "urgent",
		summary:   "Raise task urgency",
		usage:     "jczl-flow urgent <task-uuid> [urgency]",
		role:      "Use this to mark a task high priority with an urgency score.",
		effects:   []string{"Sets high priority and persists the supplied urgency value."},
		examples:  []string{"jczl-flow urgent 57c3fc80 15.0"},
		next:      "Run 'jczl-flow status' to continue with the urgent task.",
	},
	{
		name:      "block",
		group:     groupChangeReprioritize,
		canonical: "block",
		summary:   "Add a task dependency",
		usage:     "jczl-flow block <task-uuid> <dependency-uuid>",
		role:      "Use this to make one task wait for another task.",
		effects:   []string{"Adds a dependency edge and removes the task from ready work until it is satisfied."},
		examples:  []string{"jczl-flow block 57c3fc80 4f8a1d20"},
		next:      "Complete the dependency before executing the blocked task.",
	},
	{
		name:      "unblock",
		group:     groupChangeReprioritize,
		canonical: "unblock",
		summary:   "Remove a task dependency",
		usage:     "jczl-flow unblock <task-uuid> <dependency-uuid>",
		role:      "Use this to remove one dependency edge when the plan changes.",
		effects:   []string{"Removes the selected dependency without changing task completion state."},
		examples:  []string{"jczl-flow unblock 57c3fc80 4f8a1d20"},
		next:      "Run 'jczl-flow next' to inspect newly ready work.",
	},
	{
		name:      "wait",
		group:     groupChangeReprioritize,
		canonical: "wait",
		summary:   "Postpone task readiness",
		usage:     "jczl-flow wait <task-uuid> <YYYY-MM-DD|today|tomorrow>",
		role:      "Use this to keep a task out of ready work until a date.",
		effects:   []string{"Persists a wait-until date without changing completion state."},
		examples:  []string{"jczl-flow wait 57c3fc80 tomorrow"},
		next:      "Run 'jczl-flow next' after the wait date to inspect readiness.",
	},
	{
		name:      "ship",
		group:     groupStartAndOrganize,
		canonical: "roadmap",
		hidden:    true,
		summary:   "Ship a roadmap phase",
		usage:     "jczl-flow ship <roadmap-entry-id>",
		role:      "Use this compatibility alias to mark one roadmap phase as shipped.",
		effects:   []string{"Marks a pending roadmap entry completed without changing operational tasks."},
		examples:  []string{"jczl-flow ship roadmap-123"},
		next:      "Run 'jczl-flow roadmap show' to verify the shipped phase.",
	},
	{
		name:      "migrate",
		group:     groupMigrateLegacy,
		canonical: "migrate",
		summary:   "Import legacy workflow state",
		usage:     "jczl-flow migrate taskwarrior --source <export.json> [--apply]",
		role:      "Use this explicit boundary to move isolated Taskwarrior state into native Jaflow.",
		preconditions: []string{
			"Provide an explicit export snapshot; dry-run is the default.",
			"Use --apply only after reviewing the migration report.",
		},
		effects: []string{
			"Maps initiatives, UUIDs, dependencies, metadata, annotations, focus, and sessions.",
			"Never imports derived caches or calls a ticket broker.",
		},
		examples: []string{
			"jczl-flow migrate taskwarrior --source /tmp/tasks.json",
			"jczl-flow migrate taskwarrior --source /tmp/tasks.json --apply",
		},
		next: "Run the dry-run first and review retained-data warnings before applying.",
	},
	{
		name:      "cache",
		group:     groupMaintainDerived,
		canonical: "cache",
		summary:   "Inspect or clear derived output cache",
		usage:     "jczl-flow cache [info|clear [status|ponder]]",
		role:      "Use this to inspect cache entries or force a clean report boundary.",
		effects: []string{
			"Reports entries for the current project and session.",
			"Clears all cache entries or only status/ponder entries when scoped.",
		},
		examples: []string{
			"jczl-flow cache info",
			"jczl-flow cache clear status",
		},
		next: "Run 'jczl-flow status --force' or 'jczl-flow ponder --force' for a direct refresh.",
	},
}

func findHelpEntry(name string) (helpEntry, bool) {
	for _, entry := range helpEntries {
		if entry.name == name {
			return entry, true
		}
	}
	return helpEntry{}, false
}

// PrintRootHelp renders the agent-facing root help without parser-generated output.
func PrintRootHelp(writer io.Writer) {
	writeRootHelp(writer)
}

func writeRootHelp(writer io.Writer) {
	fmt.Fprintln(writer, "jczl-flow — local project workflow engine")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "ROLE")
	fmt.Fprintln(writer, "  Preserve initiatives, chained tasks, dependencies, focus, and context.")
	fmt.Fprintln(writer, "  The current command operates on one project database at a time.")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "WORKFLOW")
	fmt.Fprintln(writer, "  orient → plan → test → execute → record outcome → done → switch focus")
	fmt.Fprintln(writer, "  A task with an unfinished dependency is blocked; completion exposes the next ready task.")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "COMMANDS")
	for _, group := range NewCommandRegistry().CommonGroups() {
		fmt.Fprintln(writer, group.Name)
		for _, entry := range group.Commands {
			summary := entry.Summary
			if len(entry.Aliases) > 0 {
				summary += fmt.Sprintf(" (aliases: %s)", strings.Join(entry.Aliases, ", "))
			}
			fmt.Fprintf(writer, "  %-12s %s\n", entry.Name, summary)
		}
		fmt.Fprintln(writer, "")
	}
	fmt.Fprintln(writer, "ALIASES")
	fmt.Fprintln(writer, "  Use 'jczl-flow help <alias>' for compatibility command details.")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "AGENT RULES")
	fmt.Fprintln(writer, "  Use short UUIDs for display, but resolve and persist full UUIDs.")
	fmt.Fprintln(writer, "  Read status before acting. Treat errors and ACTION lines as workflow guidance.")
	fmt.Fprintln(writer, "  Use 'jczl-flow help <command>' for prerequisites, effects, examples, and next action.")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "NEXT")
	fmt.Fprintln(writer, "  Run 'jczl-flow status' to see the current project's pending work.")
	fmt.Fprintln(writer, "")
	writeGlobalOptions(writer)
}

func helpGroupOrder() []string {
	return []string{
		groupStartAndOrganize,
		groupExamineState,
		groupWorkCurrentTask,
		groupChangeReprioritize,
		groupPreserveContext,
		groupMaintainDerived,
		groupPrepareIntegrate,
		groupMigrateLegacy,
	}
}

func writeCommandHelp(writer io.Writer, entry CommandSpec) {
	fmt.Fprintf(writer, "jczl-flow %s — %s\n\n", entry.Name, entry.Summary)
	fmt.Fprintf(writer, "USAGE\n  %s\n\n", entry.ArgsUsage)
	fmt.Fprintf(writer, "ROLE\n  %s\n\n", entry.Description)
	writeList(writer, "PREREQUISITES", entry.Prerequisites)
	writeList(writer, "SIDE EFFECTS AND OUTPUT", entry.Effects)
	writeList(writer, "EXAMPLES", entry.Examples)
	if len(entry.Aliases) > 0 {
		writeList(writer, "ALIASES", []string{strings.Join(entry.Aliases, ", ")})
	}
	fmt.Fprintf(writer, "NEXT ACTION\n  %s\n", entry.NextAction)
}

func writeGlobalOptions(writer io.Writer) {
	fmt.Fprintln(writer, "GLOBAL OPTIONS")
	fmt.Fprintln(writer, "  -v, --verbose        Enable verbose mode")
	fmt.Fprintln(writer, "  -V, --version        Show version")
	fmt.Fprintln(writer, "      --project-id=    Project identity")
	fmt.Fprintln(writer, "      --taskdata=      Legacy Taskwarrior data directory")
	fmt.Fprintln(writer, "      --database-path= Project SQLite database path [$JACAZUL_FLOW_DATABASE_PATH]")
	fmt.Fprintln(writer, "      --session-id=    Workflow session identity [$JACAZUL_SESSION_ID]")
	fmt.Fprintln(writer, "  -h, --help           Show this help message")
}

func writeList(writer io.Writer, heading string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(writer, "%s\n", heading)
	for _, value := range values {
		fmt.Fprintf(writer, "  - %s\n", strings.TrimSpace(value))
	}
	fmt.Fprintln(writer)
}
