package cli

import (
	"fmt"
	"time"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/task"
)

// RoadmapCommand groups roadmap ledger commands.
type RoadmapCommand struct {
	Show RoadmapShowCommand `command:"show" description:"Show the roadmap ledger"`
	Init RoadmapInitCommand `command:"init" description:"Initialize the roadmap ledger"`
	Add  RoadmapAddCommand  `command:"add" description:"Add a roadmap phase"`
	Ship RoadmapShipCommand `command:"ship" description:"Ship a roadmap phase"`
}

// Execute requires a roadmap subcommand.
func (cmd *RoadmapCommand) Execute(args []string) error {
	return fmt.Errorf("roadmap requires a subcommand\nACTION: Run 'jczl-flow help roadmap'.")
}

// RoadmapShowCommand displays roadmap phases.
type RoadmapShowCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *RoadmapShowCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute shows the current ledger.
func (cmd *RoadmapShowCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("roadmap show accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	entries, err := store.ListRoadmap(cmd.appOpts.Context(), cmd.appOpts.ProjectID)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(cmd.appOpts.Out(), "No roadmap found.")
		return nil
	}
	fmt.Fprintf(cmd.appOpts.Out(), "ROADMAP: %s\n", cmd.appOpts.ProjectID)
	for _, entry := range entries {
		fmt.Fprintf(cmd.appOpts.Out(), "[%s] %s\n", entry.Phase, entry.Description)
	}
	return nil
}

// RoadmapInitCommand creates the roadmap ledger from current initiatives.
type RoadmapInitCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *RoadmapInitCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute initializes the roadmap once.
func (cmd *RoadmapInitCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("roadmap init accepts no arguments")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.InitializeRoadmap(cmd.appOpts.Context(), cmd.appOpts.ProjectID); err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Roadmap initialized: %s\n", cmd.appOpts.ProjectID)
	return nil
}

// RoadmapAddCommand adds a manually classified phase.
type RoadmapAddCommand struct {
	Phase          string `long:"phase" description:"Roadmap phase"`
	Description    string `long:"description" description:"Phase description"`
	InitiativeID   string `long:"initiative-id" description:"Optional initiative UUID"`
	InitiativeName string `long:"ini" description:"Optional initiative name"`
	appOpts        *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *RoadmapAddCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute adds a phase to the ledger.
func (cmd *RoadmapAddCommand) Execute(args []string) error {
	phase := cmd.Phase
	description := cmd.Description
	if len(args) > 0 {
		if len(args) != 2 || phase != "" || description != "" {
			return fmt.Errorf("roadmap add accepts <phase> <description> or --phase and --description\nACTION: Run 'jczl-flow help roadmap'.")
		}
		phase, description = args[0], args[1]
	}
	if phase == "" || description == "" {
		return fmt.Errorf("roadmap add requires --phase and --description\nACTION: Run 'jczl-flow help roadmap'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	initiativeID := cmd.InitiativeID
	if initiativeID == "" && cmd.InitiativeName != "" {
		initiative, err := store.FindInitiative(cmd.appOpts.Context(), cmd.appOpts.ProjectID, cmd.InitiativeName)
		if err != nil {
			return err
		}
		initiativeID = initiative.ID
	}
	entry := task.RoadmapEntry{
		ID:           newRoadmapID(),
		ProjectID:    cmd.appOpts.ProjectID,
		InitiativeID: initiativeID,
		Phase:        phase,
		Description:  description,
		Status:       task.Pending,
	}
	if err := store.AddRoadmapEntry(cmd.appOpts.Context(), entry); err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Roadmap phase added: [%s] %s (%s)\n", phase, description, entry.ID)
	return nil
}

// RoadmapShipCommand marks one roadmap phase as shipped.
type RoadmapShipCommand struct {
	appOpts *config.AppOptions
}

// SetAppOptions supplies project options to the command.
func (cmd *RoadmapShipCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute marks a roadmap phase as shipped by ID or description.
func (cmd *RoadmapShipCommand) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("roadmap ship requires one roadmap entry ID\nACTION: Run 'jczl-flow help roadmap'.")
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	entry, err := store.ShipRoadmapEntry(cmd.appOpts.Context(), cmd.appOpts.ProjectID, args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.appOpts.Out(), "Phase shipped: %s ✓\n", entry.Description)
	return nil
}

func newRoadmapID() string {
	return fmt.Sprintf("roadmap-%d", time.Now().UTC().UnixNano())
}
