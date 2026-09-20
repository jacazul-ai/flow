// Command consumer stands in for jacazul-ai-cli: a separate Go module that
// embeds the workflow engine. It imports github.com/jacazul-ai/flow and
// nothing else from the engine, so the Go toolchain refuses it any internal
// package, exactly as it will refuse jacazul.
//
// It resolves its own project, session and home, keeps the engine's output in
// its own buffer, and writes a transcript for the test to assert on. Nothing
// here reads the process environment or os.Args beyond its two arguments.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/jacazul-ai/flow"
)

var createdTask = regexp.MustCompile(`Created task ([0-9a-f]{8}): (.+)`)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: consumer <home> <transcript>")
		os.Exit(2)
	}
	home, transcript := os.Args[1], os.Args[2]

	session := workflow{
		env: flow.Env{
			ProjectID: "consumer-alpha",
			SessionID: "consumer-session",
			Home:      home,
		},
	}

	if err := session.drive(); err != nil {
		fmt.Fprintf(os.Stderr, "consumer: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(transcript, session.output.Bytes(), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "consumer: write transcript: %v\n", err)
		os.Exit(1)
	}
}

type workflow struct {
	env    flow.Env
	output bytes.Buffer
}

// run executes one engine command and returns its exit status. The engine
// writes into the consumer's buffer, never into the process streams.
func (w *workflow) run(args ...string) int {
	streams := flow.Streams{Stdout: &w.output, Stderr: &w.output}
	return flow.Run(context.Background(), args, w.env, streams)
}

func (w *workflow) mustRun(args ...string) error {
	if code := w.run(args...); code != 0 {
		return fmt.Errorf("command %v exited %d", args, code)
	}
	return nil
}

// drive walks one full task lifecycle plus a deliberate failure, which is the
// surface an embedding CLI actually needs.
func (w *workflow) drive() error {
	if err := w.mustRun("plan", "alpha", "First task", "Second task"); err != nil {
		return err
	}
	matches := createdTask.FindAllStringSubmatch(w.output.String(), -1)
	if len(matches) != 2 {
		return fmt.Errorf("plan reported %d created tasks, want 2", len(matches))
	}
	first := matches[0][1]

	for _, step := range [][]string{
		{"focus", "task", first},
		{"execute", first},
		{"outcome", first, "Done by the consumer"},
		{"done", first},
		{"status"},
	} {
		if err := w.mustRun(step...); err != nil {
			return err
		}
	}

	if code := w.run("no-such-command"); code == 0 {
		return fmt.Errorf("unknown command exited 0, want a failure status")
	}
	fmt.Fprintln(&w.output, "CONSUMER OK")
	return nil
}
