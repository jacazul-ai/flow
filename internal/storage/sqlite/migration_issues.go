package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jacazul-ai/flow/internal/task"
)

// MigrationOrigin returns the legacy source the project was migrated from,
// and false when the project database did not come from a migration.
func (s *Store) MigrationOrigin(ctx context.Context, projectID string) (task.MigrationOrigin, bool, error) {
	if projectID == "" {
		return task.MigrationOrigin{}, false, fmt.Errorf("project ID is required")
	}
	var origin task.MigrationOrigin
	err := s.db.QueryRowContext(ctx, `
		SELECT origin_path, migrated_at
		FROM migration_origins
		WHERE project_id = ?
	`, projectID).Scan(&origin.Path, &origin.MigratedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return task.MigrationOrigin{}, false, nil
	}
	if err != nil {
		return task.MigrationOrigin{}, false, fmt.Errorf("read migration origin: %w", err)
	}
	return origin, true, nil
}

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
