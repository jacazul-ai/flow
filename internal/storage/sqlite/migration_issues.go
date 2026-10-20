package sqlite

import (
	"context"
	"fmt"

	"github.com/jacazul-ai/flow/internal/task"
)

// ListMigrationIssues returns the persisted issues for one project cutoff.
func (s *Store) ListMigrationIssues(ctx context.Context, projectID string) ([]task.MigrationIssue, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, severity, message, created_at
		FROM migration_issues
		WHERE project_id = ?
		ORDER BY id
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list migration issues: %w", err)
	}
	defer rows.Close()

	var issues []task.MigrationIssue
	for rows.Next() {
		var issue task.MigrationIssue
		if err := rows.Scan(&issue.ID, &issue.ProjectID, &issue.Severity, &issue.Message, &issue.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan migration issue: %w", err)
		}
		issues = append(issues, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration issues: %w", err)
	}
	return issues, nil
}
