package cli

import (
	"fmt"

	"github.com/jacazul-ai/flow/internal/task"
)

type ticketResolution struct {
	ticket string
	found  bool
}

type ticketResolver struct {
	tasks map[string]task.Task
	memo  map[string]ticketResolution
}

func newTicketResolver(tasks []task.Task) *ticketResolver {
	byID := make(map[string]task.Task, len(tasks))
	for _, current := range tasks {
		byID[current.ID] = current
	}
	return &ticketResolver{
		tasks: byID,
		memo:  make(map[string]ticketResolution, len(tasks)),
	}
}

func (r *ticketResolver) resolve(current task.Task) (string, bool, error) {
	if current.ExternalTicket != "" {
		return current.ExternalTicket, false, nil
	}

	seen := map[string]bool{current.ID: true}
	for _, dependencyID := range current.Dependencies {
		ticket, found, err := r.resolveDependency(dependencyID, seen)
		if err != nil {
			return "", false, err
		}
		if found {
			return ticket, true, nil
		}
	}
	return "", false, nil
}

func (r *ticketResolver) resolveDependency(taskID string, seen map[string]bool) (string, bool, error) {
	if seen[taskID] {
		return "", false, nil
	}
	if cached, ok := r.memo[taskID]; ok {
		return cached.ticket, cached.found, nil
	}

	current, ok := r.tasks[taskID]
	if !ok {
		return "", false, fmt.Errorf("task %q not found", taskID)
	}
	seen[taskID] = true
	if current.ExternalTicket != "" {
		resolved := ticketResolution{ticket: current.ExternalTicket, found: true}
		r.memo[taskID] = resolved
		return resolved.ticket, resolved.found, nil
	}

	for _, dependencyID := range current.Dependencies {
		ticket, found, err := r.resolveDependency(dependencyID, seen)
		if err != nil {
			return "", false, err
		}
		if found {
			resolved := ticketResolution{ticket: ticket, found: true}
			r.memo[taskID] = resolved
			return resolved.ticket, resolved.found, nil
		}
	}
	resolved := ticketResolution{}
	r.memo[taskID] = resolved
	return resolved.ticket, resolved.found, nil
}

func tasksForInitiative(tasks []task.Task, initiativeName string) []task.Task {
	if initiativeName == "" {
		return tasks
	}
	filtered := make([]task.Task, 0, len(tasks))
	for _, current := range tasks {
		if current.InitiativeName == initiativeName {
			filtered = append(filtered, current)
		}
	}
	return filtered
}
