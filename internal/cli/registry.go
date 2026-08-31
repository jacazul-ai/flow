package cli

import "github.com/jessevdk/go-flags"

// RegisterCommands registers the CLI command tree from the help catalog.
func RegisterCommands(parser *flags.Parser) {
	registerCommand(parser, "help", NewHelpCommand(parser))

	plan := &PlanCommand{}
	registerCommand(parser, "plan", plan)
	registerCommand(parser, "initiative", plan)
	registerCommand(parser, "ini", plan)

	status := &StatusCommand{}
	registerCommand(parser, "status", status)
	registerCommand(parser, "active", &ActiveCommand{})
	registerCommand(parser, "blocked", &BlockedCommand{})
	registerCommand(parser, "overdue", &OverdueCommand{})
	registerCommand(parser, "history", &HistoryCommand{})
	registerCommand(parser, "cache", &CacheCommand{})
	registerCommand(parser, "execute", &ExecuteCommand{})
	registerCommand(parser, "next", &NextCommand{})
	registerCommand(parser, "outcome", &OutcomeCommand{})
	registerCommand(parser, "amend", &AmendCommand{})
	registerCommand(parser, "note", &NoteCommand{})
	registerCommand(parser, "notes", &NotesCommand{})
	registerCommand(parser, "context", &ContextCommand{})
	registerCommand(parser, "ticket", &TicketCommand{})
	registerCommand(parser, "handoff", &HandoffCommand{})
	registerCommand(parser, "done", &DoneCommand{})
	registerCommand(parser, "reopen", &ReopenCommand{})
	registerCommand(parser, "discard", &DiscardCommand{})
	registerCommand(parser, "rename", &RenameCommand{})
	registerCommand(parser, "urgent", &UrgentCommand{})
	registerCommand(parser, "block", &BlockCommand{})
	registerCommand(parser, "unblock", &UnblockCommand{})
	registerCommand(parser, "wait", &WaitCommand{})
	registerCommand(parser, "focus", &FocusCommand{})
	registerCommand(parser, "session", &SessionCommand{})
	registerCommand(parser, "ponder", &PonderCommand{})

	plans := &PlansCommand{}
	registerCommand(parser, "plans", plans)
	registerCommand(parser, "inis", plans)
	registerCommand(parser, "initiatives", plans)

	registerCommand(parser, "backlog", &BacklogCommand{})
	registerCommand(parser, "activate", &ActivateCommand{})
	registerCommand(parser, "tree", &TreeCommand{})
	registerCommand(parser, "commit", &CommitCommand{})
	registerCommand(parser, "roadmap", &RoadmapCommand{})
	registerCommand(parser, "ship", &RoadmapShipCommand{})
	registerCommand(parser, "migrate", &MigrateCommand{})
}

func registerCommand(parser *flags.Parser, name string, command flags.Commander) {
	entry, ok := findHelpEntry(name)
	if !ok {
		panic("command is missing from help catalog: " + name)
	}
	if entry.group == "" || entry.canonical == "" {
		panic("command is missing help taxonomy metadata: " + name)
	}
	parser.AddCommand(name, entry.summary, entry.role, command)
}
