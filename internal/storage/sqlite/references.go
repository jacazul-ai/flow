package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/task"
)

// FindInitiativeReference resolves a project-scoped initiative name or UUID reference.
func (s *Store) FindInitiativeReference(ctx context.Context, projectID string, reference string) (task.Initiative, error) {
	if projectID == "" || strings.TrimSpace(reference) == "" {
		return task.Initiative{}, fmt.Errorf("project ID and initiative reference are required")
	}
	reference = strings.TrimSpace(reference)

	exactRows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, name, status, external_ticket
		FROM initiatives
		WHERE project_id = ? AND (id = ? OR name = ?)
		ORDER BY id
	`, projectID, reference, reference)
	if err != nil {
		return task.Initiative{}, fmt.Errorf("resolve initiative reference: %w", err)
	}
	exact, err := scanInitiatives(exactRows)
	if err != nil {
		return task.Initiative{}, err
	}
	if len(exact) > 1 {
		return task.Initiative{}, fmt.Errorf("initiative reference %q is ambiguous", reference)
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(reference) < 8 {
		return task.Initiative{}, fmt.Errorf("initiative reference %q not found", reference)
	}

	allRows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, name, status, external_ticket
		FROM initiatives
		WHERE project_id = ?
		ORDER BY id
	`, projectID)
	if err != nil {
		return task.Initiative{}, fmt.Errorf("resolve initiative UUID: %w", err)
	}
	all, err := scanInitiatives(allRows)
	if err != nil {
		return task.Initiative{}, err
	}

	prefix := strings.ToLower(reference)
	matches := make([]task.Initiative, 0, 2)
	for _, initiative := range all {
		if strings.HasPrefix(strings.ToLower(initiative.ID), prefix) {
			matches = append(matches, initiative)
		}
	}
	if len(matches) > 1 {
		return task.Initiative{}, fmt.Errorf("initiative reference %q is ambiguous", reference)
	}
	if len(matches) == 0 {
		return task.Initiative{}, fmt.Errorf("initiative reference %q not found", reference)
	}
	return matches[0], nil
}

func scanInitiatives(rows *sql.Rows) ([]task.Initiative, error) {
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
			return nil, fmt.Errorf("scan initiative reference: %w", err)
		}
		initiative.Status = task.InitiativeStatus(status)
		initiatives = append(initiatives, initiative)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate initiative references: %w", err)
	}
	return initiatives, nil
}
