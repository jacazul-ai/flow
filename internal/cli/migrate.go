package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/migration"
	"github.com/jacazul-ai/flow/internal/task"
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
	return fmt.Errorf("migrate requires a subcommand\nACTION: Run 'jczl-flow help migrate'.")
}

// TaskwarriorMigrationCommand imports Taskwarrior state into native flow.
type TaskwarriorMigrationCommand struct {
	Source        string `long:"source" description:"Use an explicit Taskwarrior export JSON snapshot"`
	LegacyDataDir string `long:"legacy-data-dir" description:"Optional legacy focus/session directory"`
	Apply         bool   `long:"apply" description:"Write the migration to native SQLite"`
	DryRun        bool   `long:"dry-run" description:"Validate without writing (default)"`
	Issues        bool   `long:"issues" description:"List issues persisted by the last apply"`
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
	if cmd.Issues {
		if cmd.Apply || cmd.DryRun || strings.TrimSpace(cmd.Source) != "" || strings.TrimSpace(cmd.LegacyDataDir) != "" {
			return fmt.Errorf("--issues cannot be combined with migration input or write flags")
		}
		return cmd.listIssues()
	}
	if cmd.Apply && cmd.DryRun {
		return fmt.Errorf("--apply and --dry-run cannot be combined\nACTION: Choose --apply or omit it for the default dry-run.")
	}
	var source []migration.LegacyTask
	var err error
	if strings.TrimSpace(cmd.Source) != "" {
		source, err = migration.LoadTaskwarriorExport(cmd.Source)
	} else {
		source, err = migration.ExportTaskwarrior(cmd.appOpts.Context(), cmd.appOpts.TaskData)
	}
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
	issues := migrationIssues(warnings)
	bundle.Issues = issues
	hasErrors := printMigrationIssues(cmd.appOpts.Out(), issues)
	if !cmd.Apply {
		result, err := migration.NewImporter(nil).DryRun(cmd.appOpts.Context(), bundle)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.appOpts.Out(), "Migration dry-run: no changes written")
		printMigrationResult(cmd.appOpts.Out(), result)
		if hasErrors {
			return migrationRecordErrors()
		}
		return nil
	}

	backup, err := migration.BackupDatabase(cmd.appOpts.DatabasePath)
	if err != nil {
		return err
	}
	if backup != "" {
		fmt.Fprintf(cmd.appOpts.Out(), "Backup: %s\n", backup)
	}
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	result, err := migration.NewImporter(store).Apply(cmd.appOpts.Context(), bundle)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.appOpts.Out(), "Migration applied")
	printMigrationResult(cmd.appOpts.Out(), result)
	if hasErrors {
		return migrationRecordErrors()
	}
	return nil
}

func migrationIssues(warnings []string) []task.MigrationIssue {
	issues := make([]task.MigrationIssue, 0, len(warnings))
	for _, warning := range warnings {
		severity := "WARNING"
		message := warning
		if strings.HasPrefix(warning, "ERROR:") {
			severity = "ERROR"
			message = strings.TrimSpace(strings.TrimPrefix(warning, "ERROR:"))
		}
		issues = append(issues, task.MigrationIssue{Severity: severity, Message: message})
	}
	return issues
}

func printMigrationIssues(out io.Writer, issues []task.MigrationIssue) bool {
	hasErrors := false
	for _, issue := range issues {
		fmt.Fprintf(out, "%s: %s\n", issue.Severity, issue.Message)
		if issue.Severity == "ERROR" {
			hasErrors = true
		}
	}
	return hasErrors
}

func migrationRecordErrors() error {
	return fmt.Errorf("migration completed with record errors\nACTION: Review the ERROR entries and correct the source snapshot before rerunning")
}

func printMigrationResult(out io.Writer, result task.ImportResult) {
	fmt.Fprintf(out, "Records: created=%d updated=%d unchanged=%d dependencies=%d annotations=%d tags=%d history=%d issues=%d\n",
		result.Created, result.Updated, result.Unchanged, result.Dependencies, result.Annotations, result.Tags, result.History, result.Issues)
}

func (cmd *TaskwarriorMigrationCommand) listIssues() error {
	store, err := openStore(cmd.appOpts)
	if err != nil {
		return err
	}
	defer store.Close()
	issues, err := store.ListMigrationIssues(cmd.appOpts.Context(), cmd.appOpts.ProjectID)
	if err != nil {
		return err
	}
	if len(issues) == 0 {
		fmt.Fprintln(cmd.appOpts.Out(), "No migration issues.")
		return nil
	}
	for _, issue := range issues {
		fmt.Fprintf(cmd.appOpts.Out(), "[%s] #%d %s\n", issue.Severity, issue.ID, issue.Message)
	}
	return nil
}
