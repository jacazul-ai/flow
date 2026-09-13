package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jacazul-ai/jaflow/internal/task"
)

// ReplacePendingTaskOrder persists a complete pending-task order for one initiative.
func (s *Store) ReplacePendingTaskOrder(ctx context.Context, initiativeID string, taskIDs []string) error {
	if initiativeID == "" {
		return fmt.Errorf("initiative ID is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin task order transaction: %w", err)
	}
	defer tx.Rollback()

	current, err := loadInitiativeTaskPositions(ctx, tx, initiativeID)
	if err != nil {
		return err
	}
	pending, err := validatePendingTaskOrder(current, taskIDs)
	if err != nil {
		return err
	}
	if sameTaskOrder(pending, taskIDs) {
		return nil
	}

	var maximum int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(position), 0)
		FROM tasks
		WHERE initiative_id = ?
	`, initiativeID).Scan(&maximum); err != nil {
		return fmt.Errorf("read task order maximum: %w", err)
	}
	for index, taskID := range taskIDs {
		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks SET position = ? WHERE id = ?
		`, maximum+int64(index)+1, taskID); err != nil {
			return fmt.Errorf("stage task order: %w", err)
		}
	}

	oldPositions := make(map[string]int64, len(pending))
	for _, pendingTask := range pending {
		oldPositions[pendingTask.ID] = pendingTask.Position
	}

	now := timestamp()
	for index, taskID := range taskIDs {
		oldPosition := oldPositions[taskID]
		newPosition := pending[index].Position
		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks SET position = ?, updated_at = ? WHERE id = ?
		`, newPosition, now, taskID); err != nil {
			return fmt.Errorf("persist task order: %w", err)
		}
		if oldPosition == newPosition {
			continue
		}
		if err := appendHistoryEvent(ctx, tx, task.HistoryEvent{
			TaskID:       taskID,
			InitiativeID: initiativeID,
			EventType:    "organize",
			Property:     "position",
			OldValue:     fmt.Sprintf("%d", oldPosition),
			NewValue:     fmt.Sprintf("%d", newPosition),
			OccurredAt:   now,
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit task order: %w", err)
	}
	return nil
}

type positionedTask struct {
	ID       string
	Status   task.Status
	Position int64
}

func loadInitiativeTaskPositions(ctx context.Context, tx *sql.Tx, initiativeID string) ([]positionedTask, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, status, position
		FROM tasks
		WHERE initiative_id = ?
		ORDER BY position, id
	`, initiativeID)
	if err != nil {
		return nil, fmt.Errorf("list initiative task positions: %w", err)
	}
	defer rows.Close()

	var tasks []positionedTask
	for rows.Next() {
		var current positionedTask
		if err := rows.Scan(&current.ID, &current.Status, &current.Position); err != nil {
			return nil, fmt.Errorf("scan initiative task position: %w", err)
		}
		tasks = append(tasks, current)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate initiative task positions: %w", err)
	}
	return tasks, nil
}

func validatePendingTaskOrder(current []positionedTask, taskIDs []string) ([]positionedTask, error) {
	pending := make([]positionedTask, 0, len(current))
	for _, currentTask := range current {
		if currentTask.Status == task.Pending {
			pending = append(pending, currentTask)
		}
	}
	if len(taskIDs) != len(pending) {
		return nil, fmt.Errorf("pending task order must include every pending task exactly once")
	}

	seen := make(map[string]bool, len(taskIDs))
	pendingByID := make(map[string]positionedTask, len(pending))
	for _, currentTask := range pending {
		pendingByID[currentTask.ID] = currentTask
	}
	for _, taskID := range taskIDs {
		if seen[taskID] {
			return nil, fmt.Errorf("pending task order contains duplicate task %q", taskID)
		}
		if _, ok := pendingByID[taskID]; !ok {
			return nil, fmt.Errorf("task %q is not pending in the selected initiative", taskID)
		}
		seen[taskID] = true
	}
	return pending, nil
}

func sameTaskOrder(current []positionedTask, taskIDs []string) bool {
	if len(current) != len(taskIDs) {
		return false
	}
	for index, currentTask := range current {
		if currentTask.ID != taskIDs[index] {
			return false
		}
	}
	return true
}
