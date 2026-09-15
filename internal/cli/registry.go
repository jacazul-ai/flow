package cli

import (
	"fmt"

	"github.com/jessevdk/go-flags"
)

// CommandSpec describes one command exposed by the Jaflow CLI.
type CommandSpec struct {
	Name           string
	Summary        string
	Description    string
	Category       string
	Common         bool
	Hidden         bool
	ArgsUsage      string
	Aliases        []string
	Examples       []string
	SeeAlso        []string
	Prerequisites  []string
	Effects        []string
	NextAction     string
	Canonical      string
	HandlerFactory func() flags.Commander
}

// CommandGroup contains the commands shown under one root-help category.
type CommandGroup struct {
	Name     string
	Commands []CommandSpec
}

// CommandRegistry owns command routing and help metadata.
type CommandRegistry struct {
	specs []CommandSpec
}

var commandAliases = map[string][]string{
	"plan":  {"initiative", "ini"},
	"plans": {"inis", "initiatives"},
}

var commandFactories = map[string]func() flags.Commander{
	"help":     func() flags.Commander { return NewHelpCommand() },
	"plan":     func() flags.Commander { return &PlanCommand{} },
	"organize": func() flags.Commander { return &OrganizeCommand{} },
	"status":   func() flags.Commander { return &StatusCommand{} },
	"active":   func() flags.Commander { return &ActiveCommand{} },
	"blocked":  func() flags.Commander { return &BlockedCommand{} },
	"overdue":  func() flags.Commander { return &OverdueCommand{} },
	"history":  func() flags.Commander { return &HistoryCommand{} },
	"cache":    func() flags.Commander { return &CacheCommand{} },
	"execute":  func() flags.Commander { return &ExecuteCommand{} },
	"next":     func() flags.Commander { return &NextCommand{} },
	"outcome":  func() flags.Commander { return &OutcomeCommand{} },
	"amend":    func() flags.Commander { return &AmendCommand{} },
	"note":     func() flags.Commander { return &NoteCommand{} },
	"notes":    func() flags.Commander { return &NotesCommand{} },
	"context":  func() flags.Commander { return &ContextCommand{} },
	"ticket":   func() flags.Commander { return &TicketCommand{} },
	"handoff":  func() flags.Commander { return &HandoffCommand{} },
	"done":     func() flags.Commander { return &DoneCommand{} },
	"reopen":   func() flags.Commander { return &ReopenCommand{} },
	"discard":  func() flags.Commander { return &DiscardCommand{} },
	"rename":   func() flags.Commander { return &RenameCommand{} },
	"urgent":   func() flags.Commander { return &UrgentCommand{} },
	"block":    func() flags.Commander { return &BlockCommand{} },
	"unblock":  func() flags.Commander { return &UnblockCommand{} },
	"wait":     func() flags.Commander { return &WaitCommand{} },
	"focus":    func() flags.Commander { return &FocusCommand{} },
	"session":  func() flags.Commander { return &SessionCommand{} },
	"ponder":   func() flags.Commander { return &PonderCommand{} },
	"plans":    func() flags.Commander { return &PlansCommand{} },
	"backlog":  func() flags.Commander { return &BacklogCommand{} },
	"activate": func() flags.Commander { return &ActivateCommand{} },
	"tree":     func() flags.Commander { return &TreeCommand{} },
	"commit":   func() flags.Commander { return &CommitCommand{} },
	"roadmap":  func() flags.Commander { return &RoadmapCommand{} },
	"ship":     func() flags.Commander { return &RoadmapShipCommand{} },
	"migrate":  func() flags.Commander { return &MigrateCommand{} },
}

// NewCommandRegistry creates the canonical command registry from the help catalog.
func NewCommandRegistry() *CommandRegistry {
	specs := make([]CommandSpec, 0, len(helpEntries))
	for _, entry := range helpEntries {
		specs = append(specs, CommandSpec{
			Name:           entry.name,
			Summary:        entry.summary,
			Description:    entry.role,
			Category:       entry.group,
			Common:         !entry.hidden,
			Hidden:         entry.hidden,
			ArgsUsage:      entry.usage,
			Aliases:        cloneStrings(commandAliases[entry.name]),
			Examples:       cloneStrings(entry.examples),
			Prerequisites:  cloneStrings(entry.preconditions),
			Effects:        cloneStrings(entry.effects),
			NextAction:     entry.next,
			Canonical:      entry.canonical,
			HandlerFactory: commandFactories[entry.name],
		})
	}
	return &CommandRegistry{specs: specs}
}

// Register adds every executable command to the parser and configures aliases.
func (r *CommandRegistry) Register(parser *flags.Parser) error {
	if parser == nil {
		return fmt.Errorf("command registry requires a parser")
	}

	for _, spec := range r.specs {
		if spec.HandlerFactory == nil {
			continue
		}

		command, err := parser.AddCommand(
			spec.Name,
			spec.Summary,
			spec.Description,
			spec.HandlerFactory(),
		)
		if err != nil {
			return fmt.Errorf("register command %q: %w", spec.Name, err)
		}
		command.Hidden = spec.Hidden
		command.Aliases = cloneStrings(spec.Aliases)
	}
	return nil
}

// CommonGroups returns root-help groups in their stable display order.
func (r *CommandRegistry) CommonGroups() []CommandGroup {
	groups := make([]CommandGroup, 0, len(helpGroupOrder()))
	for _, category := range helpGroupOrder() {
		group := CommandGroup{Name: category}
		for _, spec := range r.specs {
			if spec.Common && spec.Category == category {
				group.Commands = append(group.Commands, cloneCommandSpec(spec))
			}
		}
		if len(group.Commands) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

// Find resolves a canonical command or compatibility alias.
func (r *CommandRegistry) Find(name string) (CommandSpec, bool) {
	for _, spec := range r.specs {
		if spec.Name == name {
			return cloneCommandSpec(spec), true
		}
	}
	return CommandSpec{}, false
}

// All returns command specifications, optionally including hidden aliases.
func (r *CommandRegistry) All(includeHidden bool) []CommandSpec {
	all := make([]CommandSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		if spec.Hidden && !includeHidden {
			continue
		}
		all = append(all, cloneCommandSpec(spec))
	}
	return all
}

// RegisterCommands registers the canonical command tree.
func RegisterCommands(parser *flags.Parser) error {
	return NewCommandRegistry().Register(parser)
}

func cloneCommandSpec(spec CommandSpec) CommandSpec {
	spec.Aliases = cloneStrings(spec.Aliases)
	spec.Examples = cloneStrings(spec.Examples)
	spec.SeeAlso = cloneStrings(spec.SeeAlso)
	spec.Prerequisites = cloneStrings(spec.Prerequisites)
	spec.Effects = cloneStrings(spec.Effects)
	return spec
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}
