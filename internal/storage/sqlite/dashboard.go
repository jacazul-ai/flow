package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/jacazul-ai/flow/internal/task"
)

// ListInitiatives returns dashboard summaries for one project.
func (s *Store) ListInitiatives(ctx context.Context, projectID string, includeBacklog bool, includeCompleted bool) ([]task.InitiativeSummary, error) {
	if projectID == "" {
		return nil, errors.New("project ID is required")
	}
	query := `
		SELECT id, project_id, name, status, external_ticket
		FROM initiatives
		WHERE project_id = ?
	`
	args := []any{projectID}
	if !includeBacklog {
		query += " AND status <> ?"
		args = append(args, task.InitiativeBacklog)
	}
	if !includeCompleted {
		query += " AND status <> ?"
		args = append(args, task.InitiativeCompleted)
	}
	query += " ORDER BY name"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list initiatives: %w", err)
	}
	defer rows.Close()

	var initiatives []task.Initiative
	for rows.Next() {
		var initiative task.Initiative
		var status string
		if err := rows.Scan(
			&initiative.ID,
			&initiative.ProjectID,
			&initiative.Name,
			&status,
			&initiative.ExternalTicket,
		); err != nil {
			return nil, fmt.Errorf("scan initiative: %w", err)
		}
		initiative.Status = task.InitiativeStatus(status)
		initiatives = append(initiatives, initiative)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate initiatives: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close initiative rows: %w", err)
	}

	summaries := make([]task.InitiativeSummary, 0, len(initiatives))
	for _, initiative := range initiatives {
		summary, err := s.summary(ctx, initiative)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// SetInitiativeStatus changes an initiative lifecycle marker.
func (s *Store) SetInitiativeStatus(ctx context.Context, projectID string, name string, status task.InitiativeStatus) error {
	initiative, err := s.FindInitiative(ctx, projectID, name)
	if err != nil {
		return err
	}
	if status != task.InitiativeActive && status != task.InitiativeBacklog {
		return fmt.Errorf("initiative status %q cannot be set by this command", status)
	}
	now := timestamp()
	_, err = s.db.ExecContext(ctx, `
		UPDATE initiatives SET status = ?, updated_at = ? WHERE id = ?
	`, status, now, initiative.ID)
	if err != nil {
		return fmt.Errorf("set initiative status: %w", err)
	}
	if err := s.AppendHistoryEvent(ctx, task.HistoryEvent{
		InitiativeID: initiative.ID,
		EventType:    "update",
		Property:     "status",
		OldValue:     string(initiative.Status),
		NewValue:     string(status),
		OccurredAt:   now,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Store) summary(ctx context.Context, initiative task.Initiative) (task.InitiativeSummary, error) {
	tasks, err := s.ListTasks(ctx, initiative.ProjectID, initiative.Name)
	if err != nil {
		return task.InitiativeSummary{}, err
	}
	ready, err := s.ReadyTasks(ctx, initiative.ProjectID, initiative.Name)
	if err != nil {
		return task.InitiativeSummary{}, err
	}
	readyIDs := make(map[string]struct{}, len(ready))
	for _, current := range ready {
		readyIDs[current.ID] = struct{}{}
	}

	summary := task.InitiativeSummary{Initiative: initiative}
	for _, current := range tasks {
		switch current.Status {
		case task.Pending:
			summary.Pending++
			if _, ok := readyIDs[current.ID]; !ok {
				summary.Blocked++
			}
		case task.Active:
			summary.Active++
		case task.Completed:
			summary.Completed++
		}
	}
	return summary, nil
}

func (s *Store) refreshInitiativeStatus(ctx context.Context, initiativeID string) error {
	var pending int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM tasks WHERE initiative_id = ? AND status <> ?
	`, initiativeID, task.Completed).Scan(&pending); err != nil {
		return fmt.Errorf("count initiative tasks: %w", err)
	}
	if pending > 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE initiatives SET status = ?, updated_at = ? WHERE id = ?
	`, task.InitiativeCompleted, timestamp(), initiativeID)
	if err != nil {
		return fmt.Errorf("complete initiative: %w", err)
	}
	return nil
}
