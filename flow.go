// Package flow runs the local project workflow engine.
//
// Run is the only entry point. Callers resolve the runtime context and the
// standard streams and pass them explicitly: Run never reads the process
// environment or arguments, never writes to the process streams on its own,
// and never exits the process.
package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/jacazul-ai/flow/internal/cli"
	"github.com/jacazul-ai/flow/internal/config"
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
	// Home is the runtime root used to derive default paths.
	Home string
	// Format is the default output format for report commands: text, json,
	// jsonl or xml. Empty selects text. A command's --format flag wins.
	Format string
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
	env := Env{
		ProjectID:    os.Getenv("PROJECT_ID"),
		SessionID:    os.Getenv("JACAZUL_SESSION_ID"),
		DatabasePath: os.Getenv("JACAZUL_FLOW_DATABASE_PATH"),
		Home:         os.Getenv("JACAZUL_HOME"),
		Format:       os.Getenv("JACAZUL_FLOW_FORMAT"),
	}
	if env.Home != "" {
		return env
	}
	if home, err := os.UserHomeDir(); err == nil {
		env.Home = home
	}
	return env
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

	opts := config.AppOptions{
		Ctx: ctx,
		Runtime: config.Runtime{
			ProjectID:    env.ProjectID,
			SessionID:    env.SessionID,
			DatabasePath: env.DatabasePath,
			Home:         env.Home,
			Format:       env.Format,
		},
		Stdout: stdout,
		Stderr: stderr,
	}
	parser := flags.NewParser(&opts, flags.Default&^flags.PrintErrors)
	parser.Usage = "[Options] command"
	parser.CommandHandler = config.WithAppOptions(&opts)

	if err := cli.RegisterCommands(parser); err != nil {
		fmt.Fprintf(stderr, "ERROR: register commands: %v\n", err)
		return 1
	}

	_, err := parser.ParseArgs(cli.NormalizeFocusAlias(args))
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
