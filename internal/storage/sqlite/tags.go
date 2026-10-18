package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jacazul-ai/flow/internal/task"
)

type queryContext interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listTaskTags(ctx context.Context, queryer queryContext, taskIDs []string) (map[string][]string, error) {
	tagsByTask := make(map[string][]string, len(taskIDs))
	if len(taskIDs) == 0 {
		return tagsByTask, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(taskIDs)), ",")
	args := make([]any, len(taskIDs))
	for index, taskID := range taskIDs {
		args[index] = taskID
	}
	rows, err := queryer.QueryContext(ctx, `
		SELECT task_id, tag
		FROM task_tags
		WHERE task_id IN (`+placeholders+`)
		ORDER BY task_id, tag
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list task tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var taskID, tag string
		if err := rows.Scan(&taskID, &tag); err != nil {
			return nil, fmt.Errorf("scan task tag: %w", err)
		}
		tagsByTask[taskID] = append(tagsByTask[taskID], tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task tags: %w", err)
	}
	return tagsByTask, nil
}

func replaceTaskTags(ctx context.Context, tx *sql.Tx, taskID string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_tags WHERE task_id = ?`, taskID); err != nil {
		return fmt.Errorf("clear tags for task %s: %w", taskID, err)
	}
	for _, tag := range task.NormalizeTags(tags) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_tags (task_id, tag)
			VALUES (?, ?)
		`, taskID, tag); err != nil {
			return fmt.Errorf("insert tag %q for task %s: %w", tag, taskID, err)
		}
	}
	return nil
}

// UpdateTaskTags adds and removes stored tags on one task.
func (s *Store) UpdateTaskTags(ctx context.Context, taskID string, add []string, remove []string) (task.Task, error) {
	resolvedID, err := s.resolveTaskID(ctx, taskID)
	if err != nil {
		return task.Task{}, err
	}
	add = task.NormalizeTags(add)
	remove = task.NormalizeTags(remove)
	removed := make(map[string]struct{}, len(remove))
	for _, tag := range remove {
		removed[tag] = struct{}{}
	}
	for _, tag := range add {
		if _, exists := removed[tag]; exists {
			return task.Task{}, fmt.Errorf("tag %q cannot be added and removed in one operation", tag)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return task.Task{}, fmt.Errorf("begin tag update: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), resolvedID); err != nil {
		return task.Task{}, fmt.Errorf("touch task tags: %w", err)
	}
	for _, tag := range remove {
		if _, err := tx.ExecContext(ctx, `DELETE FROM task_tags WHERE task_id = ? AND tag = ?`, resolvedID, tag); err != nil {
			return task.Task{}, fmt.Errorf("remove task tag %q: %w", tag, err)
		}
	}
	for _, tag := range add {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO task_tags (task_id, tag) VALUES (?, ?)`, resolvedID, tag); err != nil {
			return task.Task{}, fmt.Errorf("add task tag %q: %w", tag, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return task.Task{}, fmt.Errorf("commit tag update: %w", err)
	}
	return s.GetTask(ctx, resolvedID)
}

func validateStoredTags(tags []string) error {
	for _, raw := range tags {
		if strings.TrimSpace(raw) == "" {
			return errors.New("task tag cannot be empty")
		}
	}
	return nil
}
