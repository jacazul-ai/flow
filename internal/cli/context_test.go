package cli

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacazul-ai/flow/internal/config"
)

// cancelledOptions builds command options whose invocation context is already
// cancelled, against a database path that does not exist yet.
func cancelledOptions(t *testing.T) *config.AppOptions {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	return &config.AppOptions{
		Ctx:          ctx,
		ProjectID:    "project-alpha",
		SessionID:    "session-alpha",
		DatabasePath: filepath.Join(t.TempDir(), "flow.sqlite3"),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	}
}

// Commands must reach the store through the injected context. With a cancelled
// one they have to fail; a command still holding context.Background would
// happily read and write the database.
func TestCommandsUseInjectedContext(t *testing.T) {
	commands := []struct {
		name string
		args []string
		cmd  interface {
			SetAppOptions(*config.AppOptions)
			Execute([]string) error
		}
	}{
		{name: "status", cmd: &StatusCommand{}},
		{name: "next", cmd: &NextCommand{}},
		{name: "plan", args: []string{"alpha", "Alpha task"}, cmd: &PlanCommand{}},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			command.cmd.SetAppOptions(cancelledOptions(t))

			err := command.cmd.Execute(command.args)
			if err == nil {
				t.Fatal("command succeeded with a cancelled context; it is not using the injected one")
			}
			if !strings.Contains(err.Error(), "context canceled") {
				t.Fatalf("error = %v, want a cancellation failure", err)
			}
		})
	}
}

func TestAppOptionsContextDefaultsToBackground(t *testing.T) {
	opts := &config.AppOptions{}
	if opts.Context() != context.Background() {
		t.Fatal("Context() must fall back to context.Background for options built outside Run")
	}
}
