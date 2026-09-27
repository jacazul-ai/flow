// Package silo populates an isolated project database with workflow data.
//
// A silo is a project database plus the tasks, tickets and states inside it.
// Generate builds that data through flow.Run, the same boundary an agent uses,
// so a generated silo holds only states the engine itself can produce.
package silo

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	flow "github.com/jacazul-ai/flow"
)

// Chain describes one initiative whose tasks depend on each other in order.
type Chain struct {
	// Name is the initiative name.
	Name string
	// Tasks are the task descriptions in chain order. Each description must
	// be unique across the silo, because Generate keys task IDs by it.
	Tasks []string
	// Tickets maps a task index to the external ticket linked to it.
	Tickets map[int]string
	// Completed is the number of leading tasks completed with an OUTCOME.
	Completed int
	// Active starts the first task after the completed ones.
	Active bool
}

var createdTask = regexp.MustCompile(`Created task ([0-9a-f]{8}): (.+)`)

// Generate creates chains in the project selected by env and returns each
// task's short UUID keyed by its description.
func Generate(ctx context.Context, env flow.Env, chains []Chain) (map[string]string, error) {
	if err := validate(chains); err != nil {
		return nil, err
	}

	ids := make(map[string]string)
	for _, chain := range chains {
		output, err := run(ctx, env, append([]string{"plan", chain.Name}, chain.Tasks...)...)
		if err != nil {
			return nil, err
		}
		for _, match := range createdTask.FindAllStringSubmatch(output, -1) {
			ids[match[2]] = match[1]
		}
	}
	for _, chain := range chains {
		for i, description := range chain.Tasks {
			if _, ok := ids[description]; !ok {
				return nil, fmt.Errorf("silo: task %q of %q was not reported as created", description, chain.Name)
			}
			if ticket, ok := chain.Tickets[i]; ok {
				if _, err := run(ctx, env, "ticket", ids[description], ticket); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, chain := range chains {
		if err := advance(ctx, env, chain, ids); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// advance completes the leading tasks of chain and starts the next one when
// the chain asks for an active task.
func advance(ctx context.Context, env flow.Env, chain Chain, ids map[string]string) error {
	for _, description := range chain.Tasks[:chain.Completed] {
		id := ids[description]
		for _, args := range [][]string{
			{"execute", id},
			{"outcome", id, description + " is complete"},
			{"done", id},
		} {
			if _, err := run(ctx, env, args...); err != nil {
				return err
			}
		}
	}
	if !chain.Active {
		return nil
	}
	_, err := run(ctx, env, "execute", ids[chain.Tasks[chain.Completed]])
	return err
}

func validate(chains []Chain) error {
	seen := make(map[string]string)
	for _, chain := range chains {
		if chain.Name == "" || len(chain.Tasks) == 0 {
			return fmt.Errorf("silo: every chain needs a name and at least one task")
		}
		if chain.Completed < 0 || chain.Completed > len(chain.Tasks) {
			return fmt.Errorf("silo: chain %q completes %d of %d tasks", chain.Name, chain.Completed, len(chain.Tasks))
		}
		if chain.Active && chain.Completed == len(chain.Tasks) {
			return fmt.Errorf("silo: chain %q has no task left to start", chain.Name)
		}
		for i := range chain.Tickets {
			if i < 0 || i >= len(chain.Tasks) {
				return fmt.Errorf("silo: chain %q has a ticket at index %d outside its %d tasks", chain.Name, i, len(chain.Tasks))
			}
		}
		for _, description := range chain.Tasks {
			if other, ok := seen[description]; ok {
				return fmt.Errorf("silo: task %q appears in %q and %q; descriptions must be unique", description, other, chain.Name)
			}
			seen[description] = chain.Name
		}
	}
	return nil
}

func run(ctx context.Context, env flow.Env, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	streams := flow.Streams{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if code := flow.Run(ctx, args, env, streams); code != 0 {
		return "", fmt.Errorf("silo: jczl-flow %s: exit status %d: %s", strings.Join(args, " "), code, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
