package silo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	flow "github.com/jacazul-ai/flow"
)

// tickets collects repeated --ticket POSITION=TICKET flags.
type tickets map[int]string

func (t tickets) String() string { return "" }

func (t tickets) Set(value string) error {
	position, ticket, ok := strings.Cut(value, "=")
	index, err := strconv.Atoi(position)
	if !ok || err != nil || index < 1 || strings.TrimSpace(ticket) == "" {
		return fmt.Errorf("--ticket %q must be POSITION=TICKET with a position from 1, such as 2=#MID-1", value)
	}
	t[index-1] = ticket
	return nil
}

// Run is the jczl-silo command line: it generates a silo in a new database
// file and returns the process exit status.
func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("jczl-silo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	database := fs.String("database", "", "new database file to create (required; an existing file is refused)")
	project := fs.String("project", "silo", "project ID inside the database")
	session := fs.String("session", "silo", "session ID used while generating")
	chains := fs.Int("chains", 3, "number of initiatives, each a chain of dependent tasks")
	size := fs.Int("tasks", 5, "tasks per chain")
	completed := fs.Int("completed", 0, "leading tasks of each chain completed with an OUTCOME")
	active := fs.Bool("active", false, "start the first task after the completed ones in each chain")
	linked := tickets{}
	fs.Var(linked, "ticket", "link POSITION=TICKET in every chain, positions from 1 (repeatable)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, "ACTION: Run 'jczl-silo -h' for the flags.")
		return 2
	}

	env, err := newDatabaseEnv(*database, *project, *session)
	if err != nil {
		return fail(stderr, err)
	}
	if *chains < 1 || *size < 1 {
		return fail(stderr, fmt.Errorf("--chains and --tasks must be at least 1\nACTION: Pass positive counts, such as --chains 20 --tasks 20"))
	}

	spec := uniformChains(*chains, *size, *completed, *active, linked)
	if _, err := Generate(ctx, env, spec); err != nil {
		return fail(stderr, fmt.Errorf("%w\nACTION: Fix the flags and run again with a database path that does not exist", err))
	}

	fmt.Fprintf(stdout, "Generated silo %s\n", env.DatabasePath)
	fmt.Fprintf(stdout, "  project: %s | chains: %d | tasks per chain: %d | completed: %d | active: %t\n",
		env.ProjectID, *chains, *size, *completed, *active)
	fmt.Fprintf(stdout, "Inspect it with:\n  JACAZUL_PROJECT=%s JACAZUL_FLOW_DATABASE_PATH=%s jczl-flow ponder --force\n",
		env.ProjectID, env.DatabasePath)
	return 0
}

// newDatabaseEnv selects a new database file. It never falls back to the
// default project database, so the command cannot write into real data.
func newDatabaseEnv(database string, project string, session string) (flow.Env, error) {
	if strings.TrimSpace(database) == "" {
		return flow.Env{}, errors.New("--database is required\nACTION: Pass a new file path, such as --database /tmp/silo/flow.sqlite3")
	}
	path, err := filepath.Abs(database)
	if err != nil {
		return flow.Env{}, fmt.Errorf("resolve --database %q: %w", database, err)
	}
	if _, err := os.Stat(path); err == nil {
		return flow.Env{}, fmt.Errorf("%s already exists\nACTION: Pass a path that does not exist; jczl-silo only creates new databases", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return flow.Env{}, fmt.Errorf("check --database %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return flow.Env{}, fmt.Errorf("create the directory of %q: %w", path, err)
	}
	return flow.Env{ProjectID: project, SessionID: session, DatabasePath: path, Home: filepath.Dir(path)}, nil
}

// uniformChains describes count chains of size tasks with the same states.
func uniformChains(count int, size int, completed int, active bool, linked tickets) []Chain {
	width := len(strconv.Itoa(max(count, size)))
	chains := make([]Chain, 0, count)
	for c := 1; c <= count; c++ {
		name := fmt.Sprintf("chain-%0*d", width, c)
		tasks := make([]string, 0, size)
		for i := 1; i <= size; i++ {
			tasks = append(tasks, fmt.Sprintf("%s task %0*d", name, width, i))
		}
		chains = append(chains, Chain{Name: name, Tasks: tasks, Tickets: linked, Completed: completed, Active: active})
	}
	return chains
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "ERROR: %v\n", err)
	return 1
}
