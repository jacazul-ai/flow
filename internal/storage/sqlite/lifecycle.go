package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jacazul-ai/jaflow/internal/task"
)

// GetTask returns one task by UUID.
func (s *Store) GetTask(ctx context.Context, taskID string) (task.Task, error) {
	if taskID == "" {
		return task.Task{}, errors.New("task ID is required")
	}
	resolvedID, err := s.resolveTaskID(ctx, taskID)
	if err != nil {
		return task.Task{}, err
	}

	var current task.Task
	var status string
	var modeCode int64
	err = s.db.QueryRowContext(ctx, `
		SELECT t.id, t.initiative_id, i.name, t.description,
		       t.task_mode_code, t.status, t.outcome, t.external_ticket,
		       t.started_at, t.completed_at, t.disposition, t.due_at,
		       t.priority, t.urgency, t.wait_until, t.position
		FROM tasks t
		JOIN initiatives i ON i.id = t.initiative_id
		WHERE t.id = ?
	`, resolvedID).Scan(
		&current.ID,
		&current.InitiativeID,
		&current.InitiativeName,
		&current.Description,
		&modeCode,
		&status,
		&current.Outcome,
		&current.ExternalTicket,
		&current.StartedAt,
		&current.CompletedAt,
		&current.Disposition,
		&current.DueAt,
		&current.Priority,
		&current.Urgency,
		&current.WaitUntil,
		&current.Position,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, fmt.Errorf("task %q not found", taskID)
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("read task: %w", err)
	}
	current.Mode = task.TaskMode(modeCode)
	current.Status = task.Status(status)
	current.Dependencies, err = s.dependencies(ctx, current.ID)
	if err != nil {
		return task.Task{}, err
	}
	return current, nil
}

func (s *Store) resolveTaskID(ctx context.Context, input string) (string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM tasks
		WHERE id = ? OR id LIKE ?
		ORDER BY id
		LIMIT 2
	`, input, input+"%")
	if err != nil {
		return "", fmt.Errorf("resolve task ID: %w", err)
	}
	defer rows.Close()

	var matches []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", fmt.Errorf("scan task ID: %w", err)
		}
		matches = append(matches, id)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate task IDs: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("task %q not found", input)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("task ID %q is ambiguous", input)
	}
	return matches[0], nil
}

// ReadyTasks returns pending tasks whose dependencies are completed.
func (s *Store) ReadyTasks(ctx context.Context, projectID string, initiativeName string) ([]task.Task, error) {
	tasks, err := s.ListTasks(ctx, projectID, initiativeName)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]task.Task, len(tasks))
	for _, current := range tasks {
		byID[current.ID] = current
	}

	ready := make([]task.Task, 0, len(tasks))
	today := time.Now().UTC().Format("2006-01-02")
	for _, current := range tasks {
		if current.Status != task.Pending || !dependenciesCompleted(current, byID) {
			continue
		}
		if current.WaitUntil != "" && current.WaitUntil > today {
			continue
		}
		ready = append(ready, current)
	}
	return ready, nil
}

// StartTask marks a ready task active.
func (s *Store) StartTask(ctx context.Context, taskID string) error {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if current.Status == task.Completed {
		return completedTaskError(current.ID, "start")
	}
	if current.Status == task.Active {
		return nil
	}
	if current.Status != task.Pending {
		return fmt.Errorf("task %s cannot start from status %q", current.ID[:8], current.Status)
	}
	for _, dependencyID := range current.Dependencies {
		dependency, err := s.GetTask(ctx, dependencyID)
		if err != nil {
			return err
		}
		if dependency.Status != task.Completed {
			return fmt.Errorf(
				"task %s is blocked by %s\nACTION: Complete dependency %s first.",
				current.ID[:8], dependency.ID[:8], dependency.ID[:8],
			)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin start task: %w", err)
	}
	defer tx.Rollback()
	now := timestamp()
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, started_at = ?, updated_at = ?
		WHERE id = ?
	`, task.Active, now, now, current.ID); err != nil {
		return fmt.Errorf("start task: %w", err)
	}
	if err := appendHistoryEvent(ctx, tx, task.HistoryEvent{
		TaskID:       current.ID,
		InitiativeID: current.InitiativeID,
		EventType:    "start",
		Property:     "status",
		OldValue:     string(task.Pending),
		NewValue:     string(task.Active),
		OccurredAt:   now,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit start task: %w", err)
	}
	return nil
}

// RecordOutcome stores the required outcome annotation for a task.
func (s *Store) RecordOutcome(ctx context.Context, taskID string, outcome string) error {
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		return errors.New("OUTCOME cannot be empty")
	}
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if current.Status == task.Completed {
		return completedTaskError(current.ID, "record an outcome")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin outcome transaction: %w", err)
	}
	defer tx.Rollback()
	now := timestamp()
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks SET outcome = ?, updated_at = ? WHERE id = ?
	`, outcome, now, current.ID); err != nil {
		return fmt.Errorf("record task outcome: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO annotations (task_id, kind, body, created_at)
		VALUES (?, 'OUTCOME', ?, ?)
	`, current.ID, outcome, now); err != nil {
		return fmt.Errorf("record outcome annotation: %w", err)
	}
	if err := appendHistoryEvent(ctx, tx, task.HistoryEvent{
		TaskID:       current.ID,
		InitiativeID: current.InitiativeID,
		EventType:    "outcome",
		Property:     "outcome",
		OldValue:     current.Outcome,
		NewValue:     outcome,
		OccurredAt:   now,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit outcome: %w", err)
	}
	return nil
}

// CompleteTask completes a task after an outcome has been recorded.
func (s *Store) CompleteTask(ctx context.Context, taskID string) error {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if current.Status == task.Completed {
		return completedTaskError(current.ID, "complete")
	}
	if strings.TrimSpace(current.Outcome) == "" {
		return fmt.Errorf(
			"task %s cannot be completed without an OUTCOME record\nACTION: Run 'jaflow outcome %s <message>' first.",
			current.ID[:8], current.ID[:8],
		)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin complete task: %w", err)
	}
	defer tx.Rollback()
	now := timestamp()
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, task.Completed, now, now, current.ID); err != nil {
		return fmt.Errorf("complete task: %w", err)
	}
	if err := appendHistoryEvent(ctx, tx, task.HistoryEvent{
		TaskID:       current.ID,
		InitiativeID: current.InitiativeID,
		EventType:    "complete",
		Property:     "status",
		OldValue:     string(current.Status),
		NewValue:     string(task.Completed),
		OccurredAt:   now,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit complete task: %w", err)
	}
	if err := s.refreshInitiativeStatus(ctx, current.InitiativeID); err != nil {
		return err
	}
	return nil
}

// ReopenTask moves a completed task back to pending.
func (s *Store) ReopenTask(ctx context.Context, taskID string) error {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if current.Status != task.Completed {
		return fmt.Errorf("task %s is not completed", current.ID[:8])
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reopen task: %w", err)
	}
	defer tx.Rollback()
	now := timestamp()
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, completed_at = '', disposition = '', updated_at = ?
		WHERE id = ?
	`, task.Pending, now, current.ID); err != nil {
		return fmt.Errorf("reopen task: %w", err)
	}
	if err := appendHistoryEvent(ctx, tx, task.HistoryEvent{
		TaskID:       current.ID,
		InitiativeID: current.InitiativeID,
		EventType:    "reopen",
		Property:     "status",
		OldValue:     string(task.Completed),
		NewValue:     string(task.Pending),
		OccurredAt:   now,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reopen task: %w", err)
	}
	return nil
}

// DiscardTask completes a task with an auditable discard disposition.
func (s *Store) DiscardTask(ctx context.Context, taskID string) error {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if current.Status == task.Completed {
		return completedTaskError(current.ID, "discard")
	}

	const audit = "Task discarded and moved to archive."
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin discard transaction: %w", err)
	}
	defer tx.Rollback()
	now := timestamp()
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, disposition = 'discarded', outcome = ?,
		    completed_at = ?, updated_at = ?
		WHERE id = ?
	`, task.Completed, audit, now, now, current.ID); err != nil {
		return fmt.Errorf("discard task: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO annotations (task_id, kind, body, created_at)
		VALUES (?, 'OUTCOME', ?, ?)
	`, current.ID, audit, now); err != nil {
		return fmt.Errorf("record discard audit: %w", err)
	}
	if err := appendHistoryEvent(ctx, tx, task.HistoryEvent{
		TaskID:       current.ID,
		InitiativeID: current.InitiativeID,
		EventType:    "discard",
		Property:     "disposition",
		OldValue:     current.Disposition,
		NewValue:     "discarded",
		OccurredAt:   now,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit discard: %w", err)
	}
	return nil
}

func dependenciesCompleted(current task.Task, tasks map[string]task.Task) bool {
	for _, dependencyID := range current.Dependencies {
		dependency, ok := tasks[dependencyID]
		if !ok || dependency.Status != task.Completed {
			return false
		}
	}
	return true
}

func completedTaskError(taskID string, operation string) error {
	shortID := taskID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	return fmt.Errorf(
		"task %s is already COMPLETED; cannot %s\nACTION: Use 'jaflow amend %s ...' for metadata or 'jaflow reopen %s' for more work.",
		shortID, operation, shortID, shortID,
	)
}
