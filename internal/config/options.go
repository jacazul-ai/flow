package config

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/jessevdk/go-flags"
)

var ErrVersionRequired = errors.New("version required")

// Database locations derived under the runtime home. The legacy pair belongs
// to the releases that shipped before the engine was renamed to flow.
const (
	databaseDirectory       = "flow"
	databaseFile            = "flow.sqlite3"
	legacyDatabaseDirectory = "jaflow"
	legacyDatabaseFile      = "jaflow.sqlite3"
)

// ErrHomeRequired reports that default paths cannot be derived without a runtime home.
var ErrHomeRequired = errors.New("runtime home is required\nACTION: Set JACAZUL_HOME or pass a home directory to flow.Run.")

type AppOptions struct {
	Verbose      bool   `short:"v" long:"verbose" description:"Enable verbose mode"`
	Version      bool   `short:"V" long:"version" description:"Show version"`
	ProjectID    string `long:"project-id" description:"Project identity"`
	TaskData     string `long:"taskdata" description:"Legacy Taskwarrior data directory"`
	DatabasePath string `long:"database-path" description:"Project SQLite database path"`
	SessionID    string `long:"session-id" description:"Workflow session identity"`

	// LegacyDatabasePath is the pre-rename location migrated on first open.
	// It is derived only when the database path itself was derived.
	LegacyDatabasePath string `no-flag:"true"`

	// Ctx is the invocation context. go-flags fixes Execute to
	// (args []string) error, so there is no parameter to thread it through;
	// AppOptions is the per-invocation handoff that carries it instead.
	Ctx context.Context `no-flag:"true"`
	// Runtime is the caller-resolved context used when a flag is not supplied.
	Runtime Runtime `no-flag:"true"`
	// Stdout and Stderr receive command output.
	Stdout io.Writer `no-flag:"true"`
	Stderr io.Writer `no-flag:"true"`
}

// Runtime is the invocation context resolved by the caller, never read from
// the process environment inside the engine.
type Runtime struct {
	ProjectID    string
	SessionID    string
	DatabasePath string
	Home         string
}

type AppOptionsAware interface {
	SetAppOptions(opts *AppOptions)
}

type AppOptionsFunc func(opts *AppOptions) error

func WithAppOptions(opts *AppOptions, fns ...AppOptionsFunc) func(
	cmd flags.Commander, args []string) error {
	return func(cmd flags.Commander, args []string) error {
		if opts.Version {
			return ErrVersionRequired
		}
		if err := Resolve(opts); err != nil {
			return err
		}

		if len(fns) > 0 {
			for _, fn := range fns {
				if err := fn(opts); err != nil {
					return err
				}
			}
		}

		if aware, ok := cmd.(AppOptionsAware); ok == true {
			aware.SetAppOptions(opts)
		}
		return cmd.Execute(args)
	}
}

// Resolve fills project-scoped options from flags first, then the runtime context.
func Resolve(opts *AppOptions) error {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.ProjectID == "" {
		opts.ProjectID = opts.Runtime.ProjectID
	}
	if opts.ProjectID == "" {
		opts.ProjectID = "global"
	}
	if opts.SessionID == "" {
		opts.SessionID = opts.Runtime.SessionID
	}
	if opts.SessionID == "" {
		opts.SessionID = "global"
	}
	if opts.DatabasePath == "" {
		opts.DatabasePath = opts.Runtime.DatabasePath
	}
	if opts.TaskData != "" && opts.DatabasePath != "" {
		return nil
	}

	home := opts.Runtime.Home
	if home == "" {
		return ErrHomeRequired
	}
	if opts.TaskData == "" {
		opts.TaskData = filepath.Join(home, ".task", opts.ProjectID)
	}
	if opts.DatabasePath == "" {
		opts.DatabasePath = filepath.Join(
			home,
			databaseDirectory,
			opts.ProjectID,
			databaseFile,
		)
		opts.LegacyDatabasePath = filepath.Join(
			home,
			legacyDatabaseDirectory,
			opts.ProjectID,
			legacyDatabaseFile,
		)
	}
	return nil
}

// Out returns the stream for command output. It is never nil, so commands
// constructed outside Resolve still write somewhere harmless.
func (opts *AppOptions) Out() io.Writer {
	if opts == nil || opts.Stdout == nil {
		return io.Discard
	}
	return opts.Stdout
}

// Err returns the stream for diagnostics. It is never nil, for the same
// reason as Out.
func (opts *AppOptions) Err() io.Writer {
	if opts == nil || opts.Stderr == nil {
		return io.Discard
	}
	return opts.Stderr
}

// Context returns the invocation context. It is never nil, so a command built
// outside Run still has a context to pass to the store.
func (opts *AppOptions) Context() context.Context {
	if opts == nil || opts.Ctx == nil {
		return context.Background()
	}
	return opts.Ctx
}
