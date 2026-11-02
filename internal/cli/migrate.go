package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/migration"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jacazul-ai/flow/internal/task"
)

// MigrateCommand groups explicit legacy migration commands.
type MigrateCommand struct {
	Taskwarrior TaskwarriorMigrationCommand `command:"taskwarrior" description:"Import legacy Taskwarrior state"`
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
	LegacyMap     string `long:"legacy-map" description:"Optional legacy annotation and mode mapping JSON"`
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

// Execute validates and optionally applies one Taskwarrior migration.
func (cmd *TaskwarriorMigrationCommand) Execute(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("migrate taskwarrior accepts flags only")
	}
	if cmd.Issues {
		if cmd.Apply || cmd.DryRun || strings.TrimSpace(cmd.Source) != "" || strings.TrimSpace(cmd.LegacyMap) != "" || strings.TrimSpace(cmd.LegacyDataDir) != "" {
			return fmt.Errorf("--issues cannot be combined with migration input or write flags")
		}
		return cmd.listIssues()
	}
	if cmd.Apply && cmd.DryRun {
		return fmt.Errorf("--apply and --dry-run cannot be combined\nACTION: Choose --apply or omit it for the default dry-run.")
	}
	legacyMap, err := migration.LoadLegacyMap(cmd.LegacyMap)
	if err != nil {
		return err
	}
	var source []migration.LegacyTask
	if strings.TrimSpace(cmd.Source) != "" {
		source, err = migration.LoadTaskwarriorExport(cmd.Source)
	} else {
		source, err = migration.ExportTaskwarrior(cmd.appOpts.Context(), cmd.appOpts.TaskData, cmd.appOpts.TaskBinary)
	}
	if err != nil {
		return err
	}
	legacyState, err := migration.LoadLegacyState(cmd.LegacyDataDir)
	if err != nil {
		return err
	}
	bundle, warnings, err := migration.BuildBundleWithStateAndMap(cmd.appOpts.ProjectID, source, legacyState, legacyMap)
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
	return fmt.Errorf("%w\nACTION: Review the ERROR entries and correct the source snapshot before rerunning", migration.ErrRecordErrors)
}

// MigrationRequest is one library migration from a Taskwarrior export.
type MigrationRequest struct {
	Source        string
	Origin        string
	LegacyDataDir string
	Apply         bool
}

// MigrationOutcome is what a library migration validated or wrote.
type MigrationOutcome struct {
	Applied bool
	Records task.ImportResult
	Issues  []task.MigrationIssue
}

// MigrateTaskwarrior validates a Taskwarrior export and, on Apply, writes it
// into a new project database together with its origin mark. It never writes
// to an existing database and prints nothing. ERROR issues fill the outcome and
// also return migration.ErrRecordErrors.
func MigrateTaskwarrior(opts *config.AppOptions, request MigrationRequest) (MigrationOutcome, error) {
	ctx := opts.Context()
	if err := ctx.Err(); err != nil {
		return MigrationOutcome{}, err
	}
	if strings.TrimSpace(request.Source) == "" || strings.TrimSpace(request.Origin) == "" {
		return MigrationOutcome{}, fmt.Errorf("migration requires a source export and its origin\nACTION: Pass the Taskwarrior export path as Source and the silo it came from as Origin.")
	}
	if err := refuseExistingDatabase(opts); err != nil {
		return MigrationOutcome{}, err
	}
	source, err := migration.LoadTaskwarriorExport(request.Source)
	if err != nil {
		return MigrationOutcome{}, err
	}
	legacyState, err := migration.LoadLegacyState(request.LegacyDataDir)
	if err != nil {
		return MigrationOutcome{}, err
	}
	bundle, warnings, err := migration.BuildBundleWithStateAndMap(opts.ProjectID, source, legacyState, migration.LegacyMap{})
	if err != nil {
		return MigrationOutcome{}, err
	}
	outcome := MigrationOutcome{Applied: request.Apply, Issues: migrationIssues(warnings)}
	bundle.Issues = outcome.Issues
	bundle.OriginPath = request.Origin

	if request.Apply {
		outcome.Records, err = applyToNewDatabase(ctx, opts.DatabasePath, bundle)
	} else {
		outcome.Records, err = migration.NewImporter(nil).DryRun(ctx, bundle)
	}
	if err != nil {
		return MigrationOutcome{}, err
	}
	if hasRecordErrors(outcome.Issues) {
		return outcome, migrationRecordErrors()
	}
	return outcome, nil
}

func refuseExistingDatabase(opts *config.AppOptions) error {
	for _, path := range []string{opts.DatabasePath, opts.LegacyDatabasePath} {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%w: %s\nACTION: Use the existing database, or move it away before migrating again.", migration.ErrDatabaseExists, path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("inspect project database %s: %w", path, err)
		}
	}
	return nil
}

// applyToNewDatabase builds the database at a staging path and links it into
// place only after the import committed, so a failed or interrupted apply
// never leaves a database behind that would block the next migration.
func applyToNewDatabase(ctx context.Context, path string, bundle task.ImportBundle) (task.ImportResult, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return task.ImportResult{}, fmt.Errorf("create project database directory: %w", err)
	}
	staging := path + ".migrating"
	removeDatabaseFiles(staging)
	defer removeDatabaseFiles(staging)

	store, err := sqlite.Open(ctx, staging)
	if err != nil {
		return task.ImportResult{}, fmt.Errorf("create project database: %w", err)
	}
	result, err := migration.NewImporter(store).Apply(ctx, bundle)
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return task.ImportResult{}, err
	}
	if err := os.Link(staging, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return task.ImportResult{}, fmt.Errorf("%w: %s\nACTION: Use the existing database, or move it away before migrating again.", migration.ErrDatabaseExists, path)
		}
		return task.ImportResult{}, fmt.Errorf("place project database %s: %w", path, err)
	}
	return result, nil
}

func removeDatabaseFiles(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(path + suffix)
	}
}

func hasRecordErrors(issues []task.MigrationIssue) bool {
	for _, issue := range issues {
		if issue.Severity == "ERROR" {
			return true
		}
	}
	return false
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
