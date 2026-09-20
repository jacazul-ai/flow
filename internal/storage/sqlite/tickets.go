package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/task"
)

// FindExternalTicket resolves a task ticket, recursively checking dependencies.
// The inherited result reports whether the ticket came from an ancestor.
func (s *Store) FindExternalTicket(ctx context.Context, taskID string) (ticket string, inherited bool, err error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return "", false, err
	}
	if current.ExternalTicket != "" {
		return current.ExternalTicket, false, nil
	}

	seen := map[string]bool{current.ID: true}
	for _, dependencyID := range current.Dependencies {
		ticket, found, err := s.findExternalTicket(ctx, dependencyID, seen)
		if err != nil {
			return "", false, err
		}
		if found {
			return ticket, true, nil
		}
	}
	return "", false, nil
}

func (s *Store) findExternalTicket(ctx context.Context, taskID string, seen map[string]bool) (string, bool, error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return "", false, err
	}
	if seen[current.ID] {
		return "", false, nil
	}
	seen[current.ID] = true
	if current.ExternalTicket != "" {
		return current.ExternalTicket, true, nil
	}
	for _, dependencyID := range current.Dependencies {
		ticket, found, err := s.findExternalTicket(ctx, dependencyID, seen)
		if err != nil {
			return "", false, err
		}
		if found {
			return ticket, true, nil
		}
	}
	return "", false, nil
}

// SetTaskTicket stores a direct external ticket on a task.
func (s *Store) SetTaskTicket(ctx context.Context, taskID string, ticket string) error {
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return errors.New("ticket is required")
	}
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if current.Status == task.Completed {
		return completedTaskError(current.ID, "set a ticket")
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE tasks SET external_ticket = ?, updated_at = ? WHERE id = ?
	`, ticket, timestamp(), current.ID)
	if err != nil {
		return fmt.Errorf("set task ticket: %w", err)
	}
	return nil
}
