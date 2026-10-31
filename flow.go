// Package flow runs the local project workflow engine.
//
// Run is the command entry point. Init and MigrateTaskwarrior are the library
// calls that settle storage before a command. Callers resolve the runtime
// context and the standard streams and pass them explicitly: no entry point
// reads the process environment or arguments, writes to the process streams on
// its own, or exits the process.
package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/jacazul-ai/flow/internal/cli"
	"github.com/jacazul-ai/flow/internal/config"
	"github.com/jacazul-ai/flow/internal/migration"
	"github.com/jacazul-ai/flow/internal/storage/sqlite"
	"github.com/jessevdk/go-flags"
)

const modulePath = "github.com/jacazul-ai/flow"

// Env is the resolved runtime context for one invocation.
type Env struct {
	// ProjectID scopes workflow state. Empty selects the "global" project.
	ProjectID string
	// SessionID scopes focus and cache. Empty selects the "global" session.
	SessionID string
	// DatabasePath overrides the default project database location.
	DatabasePath string
	// TaskBinary is the explicit Taskwarrior executable for migrations. When
	// empty, standalone execution resolves task through PATH.
	TaskBinary string
	// Home is the runtime root used to derive default paths.
	Home string
	// Format is the default format for report commands: text, json,
	// jsonl or xml. Empty selects text. A command's --format flag wins.
	Format string
	// Sources identifies where standalone runtime values came from.
	Sources RuntimeSources
}

// RuntimeSources identifies the source of resolved standalone runtime values.
type RuntimeSources struct {
	ProjectID    string
	SessionID    string
	DatabasePath string
	Home         string
	Format       string
}

// Streams are the standard streams of one invocation.
type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// EnvFromOS builds Env from the process environment for standalone
// executables. Embedding callers build Env from their own resolved context.
func EnvFromOS() Env {
	home := os.Getenv(config.EnvHome)
	homeSource := "environment"
	if home == "" {
		homeSource = "default"
		if userHome, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(userHome, config.DefaultHomeDir)
		}
	}

	project := os.Getenv(config.EnvProject)
	projectSource := "environment"
	if project == "" {
		projectSource = "default"
		project = config.ResolveProjectID("")
	}

	session := os.Getenv(config.EnvSession)
	sessionSource := "environment"
	if session == "" {
		sessionSource = "default"
	}
	databasePath := os.Getenv("JACAZUL_FLOW_DATABASE_PATH")
	databaseSource := "environment"
	if databasePath == "" {
		databaseSource = "default"
	}
	format := os.Getenv("JACAZUL_FLOW_FORMAT")
	formatSource := "environment"
	if format == "" {
		formatSource = "default"
	}

	return Env{
		ProjectID:    project,
		SessionID:    session,
		DatabasePath: databasePath,
		Home:         home,
		Format:       format,
		Sources: RuntimeSources{
			ProjectID:    projectSource,
			SessionID:    sessionSource,
			DatabasePath: databaseSource,
			Home:         homeSource,
			Format:       formatSource,
		},
	}
}

// Run executes one command with args and returns its exit status.
func Run(ctx context.Context, args []string, env Env, streams Streams) int {
	stdout := writerOrDiscard(streams.Stdout)
	stderr := writerOrDiscard(streams.Stderr)
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if len(args) == 0 {
		args = []string{"help"}
	}

	opts := appOptions(ctx, env, stdout, stderr)
	parser := flags.NewParser(&opts, flags.Default&^flags.PrintErrors)
	parser.Usage = "[Options] command"
	parser.CommandHandler = config.WithAppOptions(&opts)

	if err := cli.RegisterCommands(parser); err != nil {
		fmt.Fprintf(stderr, "ERROR: register commands: %v\n", err)
		return 1
	}

	args = cli.NormalizeFocusAlias(args)
	args = cli.NormalizeTagPredicates(args)
	_, err := parser.ParseArgs(args)
	if err == nil {
		return 0
	}
	if errors.Is(err, config.ErrVersionRequired) {
		fmt.Fprintln(stdout, version())
		return 0
	}
	var flagsErr *flags.Error
	if !errors.As(err, &flagsErr) {
		// The command ran and failed. Its error already names the next valid
		// move, so the parser usage would only bury it.
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if flagsErr.Type == flags.ErrHelp {
		cli.PrintRootHelp(stdout)
		return 0
	}

	fmt.Fprintf(stderr, "ERROR: %v\n", err)
	fmt.Fprintln(stderr, "ACTION: Review the command syntax or run 'jczl-flow help'.")
	parser.WriteHelp(stderr)
	return 1
}

// ErrSchemaNewer classifies an Init or command failure caused by a database
// whose schema a newer release wrote. The error message carries the ACTION.
var ErrSchemaNewer = sqlite.ErrSchemaNewer

// ErrDatabaseConflict classifies an Init or command failure caused by a
// database at both the legacy and the current path. The error message carries
// the ACTION.
var ErrDatabaseConflict = sqlite.ErrDatabaseConflict

// InitResult describes what Init changed in the project database.
//
// A database is current when neither Created nor Upgraded is set; From and To
// are then equal. Moved can combine with Upgraded, and LegacyPath is set only
// when Moved is.
type InitResult struct {
	// Path is the project database Init settled.
	Path string
	// LegacyPath is where the database was before Init moved it.
	LegacyPath string
	// Moved reports that the database was relocated from LegacyPath.
	Moved bool
	// Created reports that the database did not exist before Init.
	Created bool
	// Upgraded reports that Init applied pending schema migrations.
	Upgraded bool
	// From is the schema version found, and To the version left in place.
	From, To int64
	// Origin is the legacy source the database was migrated from by
	// MigrateTaskwarrior, or nil when it did not come from such a migration.
	Origin *MigrationOrigin
}

// MigrationOrigin is the origin mark MigrateTaskwarrior records with the
// imported records.
type MigrationOrigin struct {
	// Path is the Origin the caller declared, recorded verbatim.
	Path string
	// MigratedAt is when the migration committed, in UTC.
	MigratedAt time.Time
}

// Init settles the project database the way the init command does: it moves a
// database left at the legacy path, creates a missing one, or applies pending
// schema migrations. It writes no initiatives, tasks, sessions, focus, or cache
// state and prints nothing; the caller renders the result. Env resolves the
// database path exactly as it does for Run.
func Init(ctx context.Context, env Env) (InitResult, error) {
	if err := ctx.Err(); err != nil {
		return InitResult{}, err
	}
	opts := appOptions(ctx, env, nil, nil)
	if err := config.Resolve(&opts); err != nil {
		return InitResult{}, err
	}
	result, err := cli.InitStorage(&opts)
	if err != nil {
		return InitResult{}, err
	}
	initResult := InitResult{
		Path:       result.Path,
		LegacyPath: result.LegacyPath,
		Moved:      result.Moved,
		Created:    result.Created,
		Upgraded:   result.Upgraded,
		From:       result.From,
		To:         result.To,
	}
	if result.Origin != nil {
		migratedAt, err := time.Parse(time.RFC3339Nano, result.Origin.MigratedAt)
		if err != nil {
			return InitResult{}, fmt.Errorf("read migration origin time %q: %w", result.Origin.MigratedAt, err)
		}
		initResult.Origin = &MigrationOrigin{Path: result.Origin.Path, MigratedAt: migratedAt}
	}
	return initResult, nil
}

// ErrDatabaseExists classifies a MigrateTaskwarrior call refused because the
// project already has a database, at the current or the legacy path. Nothing
// is written. The error message carries the ACTION.
var ErrDatabaseExists = migration.ErrDatabaseExists

// ErrMigrationRecordErrors classifies a MigrateTaskwarrior call that finished
// with ERROR issues; its MigrateResult is filled. The error message carries
// the ACTION.
var ErrMigrationRecordErrors = migration.ErrRecordErrors

// MigrateOptions selects the export a MigrateTaskwarrior call reads.
type MigrateOptions struct {
	// Source is the path of a Taskwarrior export JSON file.
	Source string
	// Origin is the legacy source the export came from. It is recorded
	// verbatim with the imported records and returned by Init, so the caller
	// compares it with the same canonical value it declared.
	Origin string
	// Apply writes the migration; false validates it as a dry-run.
	Apply bool
}

// MigrateCounts counts the records a migration validated or wrote.
type MigrateCounts struct {
	Created, Updated, Unchanged     int
	Dependencies, Annotations, Tags int
	History, Issues                 int
}

// MigrateIssue is one WARNING or ERROR found while building the migration.
type MigrateIssue struct {
	Severity string
	Message  string
}

// MigrateResult describes what MigrateTaskwarrior validated or wrote.
type MigrateResult struct {
	// Applied reports that the records and the origin mark were written.
	Applied bool
	Records MigrateCounts
	Issues  []MigrateIssue
}

// MigrateTaskwarrior migrates a Taskwarrior export into a new project
// database. A dry-run creates nothing. Apply creates the database and writes
// the records and the origin mark in one transaction, and leaves no database
// behind when it fails. It never writes to an existing database and prints
// nothing; the caller renders the result. Env resolves the database path
// exactly as it does for Run.
func MigrateTaskwarrior(ctx context.Context, env Env, options MigrateOptions) (MigrateResult, error) {
	if err := ctx.Err(); err != nil {
		return MigrateResult{}, err
	}
	opts := appOptions(ctx, env, nil, nil)
	if err := config.Resolve(&opts); err != nil {
		return MigrateResult{}, err
	}
	outcome, err := cli.MigrateTaskwarrior(&opts, cli.MigrationRequest{
		Source: options.Source,
		Origin: options.Origin,
		Apply:  options.Apply,
	})
	result := MigrateResult{
		Applied: outcome.Applied,
		Records: MigrateCounts{
			Created:      outcome.Records.Created,
			Updated:      outcome.Records.Updated,
			Unchanged:    outcome.Records.Unchanged,
			Dependencies: outcome.Records.Dependencies,
			Annotations:  outcome.Records.Annotations,
			Tags:         outcome.Records.Tags,
			History:      outcome.Records.History,
			Issues:       outcome.Records.Issues,
		},
	}
	for _, issue := range outcome.Issues {
		result.Issues = append(result.Issues, MigrateIssue{Severity: issue.Severity, Message: issue.Message})
	}
	return result, err
}

// appOptions builds the options every entry point resolves from Env.
func appOptions(ctx context.Context, env Env, stdout io.Writer, stderr io.Writer) config.AppOptions {
	return config.AppOptions{
		Ctx: ctx,
		Runtime: config.Runtime{
			ProjectID:      env.ProjectID,
			SessionID:      env.SessionID,
			DatabasePath:   env.DatabasePath,
			TaskBinary:     env.TaskBinary,
			Home:           env.Home,
			Format:         env.Format,
			ProjectSource:  env.Sources.ProjectID,
			SessionSource:  env.Sources.SessionID,
			DatabaseSource: env.Sources.DatabasePath,
			HomeSource:     env.Sources.Home,
			FormatSource:   env.Sources.Format,
			Version:        version(),
		},
		Stdout: stdout,
		Stderr: stderr,
	}
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

// version reports this module's version, whether it is the main module of a
// standalone executable or a dependency embedded in another program.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if info.Main.Path == modulePath && info.Main.Version != "" {
		return info.Main.Version
	}
	for _, dep := range info.Deps {
		if dep.Path != modulePath {
			continue
		}
		if dep.Replace != nil && dep.Replace.Version != "" {
			return dep.Replace.Version
		}
		if dep.Version != "" {
			return dep.Version
		}
	}
	return "dev"
}
