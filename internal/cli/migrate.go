package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/jacazul-ai/jaflow/internal/config"
	"github.com/jacazul-ai/jaflow/internal/migration"
	"github.com/jacazul-ai/jaflow/internal/task"
)

// MigrateCommand groups explicit legacy migration commands.
type MigrateCommand struct {
	Taskwarrior TaskwarriorMigrationCommand `command:"taskwarrior" description:"Import a Taskwarrior export snapshot"`
}

// SetAppOptions supplies project-scoped options to migration commands.
func (cmd *MigrateCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.Taskwarrior.SetAppOptions(opts)
}

// Execute requires a migration source subcommand.
func (cmd *MigrateCommand) Execute(args []string) error {
	return fmt.Errorf("migrate requires a subcommand\nACTION: Run 'jaflow help migrate'.")
}

// TaskwarriorMigrationCommand imports one explicit Taskwarrior snapshot.
type TaskwarriorMigrationCommand struct {
	Source        string `long:"source" description:"Taskwarrior export JSON path"`
	LegacyDataDir string `long:"legacy-data-dir" description:"Optional legacy focus/session directory"`
	Apply         bool   `long:"apply" description:"Write the migration to native SQLite"`
	DryRun        bool   `long:"dry-run" description:"Validate without writing (default)"`
	appOpts       *config.AppOptions
}

// SetAppOptions supplies project-scoped options to the command.
func (cmd *TaskwarriorMigrationCommand) SetAppOptions(opts *config.AppOptions) {
	cmd.appOpts = opts
}

// Execute validates and optionally applies one migration snapshot.
func (cmd *TaskwarriorMigrationCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("migrate taskwarrior accepts flags only")
	}
	if strings.TrimSpace(cmd.Source) == "" {
		return fmt.Errorf("migration source is required\nACTION: Run 'jaflow migrate taskwarrior --source <export.json>'.")
	}
	if cmd.Apply && cmd.DryRun {
		return fmt.Errorf("--apply and --dry-run cannot be combined\nACTION: Choose --apply or omit it for the default dry-run.")
	}
	source, err := migration.LoadTaskwarriorExport(cmd.Source)
	if err != nil {
		return err
	}
	legacyState, err := migration.LoadLegacyState(cmd.LegacyDataDir)
	if err != nil {
		return err
	}
	bundle, warnings, err := migration.BuildBundleWithState(cmd.appOpts.ProjectID, source, legacyState)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Printf("WARNING: %s\n", warning)
	}
	if !cmd.Apply {
		result, err := migration.NewImporter(nil).DryRun(context.Background(), bundle)
		if err != nil {
			return err
		}
		fmt.Println("Migration dry-run: no changes written")
		printMigrationResult(result)
		return nil
	}

	backup, err := migration.BackupDatabase(cmd.appOpts.DatabasePath)
	if err != nil {
		return err
	}
	if backup != "" {
		fmt.Printf("Backup: %s\n", backup)
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	result, err := migration.NewImporter(store).Apply(context.Background(), bundle)
	if err != nil {
		return err
	}
	fmt.Println("Migration applied")
	printMigrationResult(result)
	return nil
}

func printMigrationResult(result task.ImportResult) {
	fmt.Printf("Records: created=%d updated=%d unchanged=%d dependencies=%d annotations=%d history=%d\n",
		result.Created, result.Updated, result.Unchanged, result.Dependencies, result.Annotations, result.History)
}
