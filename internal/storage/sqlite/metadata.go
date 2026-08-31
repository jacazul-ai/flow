package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jacazul-ai/jaflow/internal/task"
)

// UpdateTaskMetadata amends the supplied non-nil task metadata fields.
func (s *Store) UpdateTaskMetadata(ctx context.Context, taskID string, update task.TaskMetadataUpdate) (task.Task, error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return task.Task{}, err
	}

	sets := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if update.Description != nil {
		description := strings.TrimSpace(*update.Description)
		if description == "" {
			return task.Task{}, errors.New("task description cannot be empty")
		}
		sets = append(sets, "description = ?")
		args = append(args, description)
	}
	if update.ExternalTicket != nil {
		sets = append(sets, "external_ticket = ?")
		args = append(args, strings.TrimSpace(*update.ExternalTicket))
	}
	if len(sets) == 0 {
		return task.Task{}, errors.New("no task metadata fields supplied")
	}

	sets = append(sets, "updated_at = ?")
	now := timestamp()
	args = append(args, now, current.ID)
	query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = ?", strings.Join(sets, ", "))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return task.Task{}, fmt.Errorf("begin amend task metadata: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return task.Task{}, fmt.Errorf("amend task metadata: %w", err)
	}
	if update.Description != nil {
		if err := appendHistoryEvent(ctx, tx, task.HistoryEvent{
			TaskID:       current.ID,
			InitiativeID: current.InitiativeID,
			EventType:    "update",
			Property:     "description",
			OldValue:     current.Description,
			NewValue:     strings.TrimSpace(*update.Description),
			OccurredAt:   now,
		}); err != nil {
			return task.Task{}, err
		}
	}
	if update.ExternalTicket != nil {
		if err := appendHistoryEvent(ctx, tx, task.HistoryEvent{
			TaskID:       current.ID,
			InitiativeID: current.InitiativeID,
			EventType:    "update",
			Property:     "external_ticket",
			OldValue:     current.ExternalTicket,
			NewValue:     strings.TrimSpace(*update.ExternalTicket),
			OccurredAt:   now,
		}); err != nil {
			return task.Task{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return task.Task{}, fmt.Errorf("commit amended metadata: %w", err)
	}
	return s.GetTask(ctx, current.ID)
}

// RenameInitiative changes an initiative name within one project.
func (s *Store) RenameInitiative(ctx context.Context, projectID string, oldName string, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if projectID == "" || oldName == "" || newName == "" {
		return errors.New("project ID and initiative names are required")
	}
	initiative, err := s.FindInitiative(ctx, projectID, oldName)
	if err != nil {
		return err
	}
	if oldName == newName {
		return nil
	}

	var existingID string
	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM initiatives WHERE project_id = ? AND name = ?
	`, projectID, newName).Scan(&existingID)
	if err == nil {
		return fmt.Errorf("initiative %q already exists", newName)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check initiative name: %w", err)
	}

	now := timestamp()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE initiatives SET name = ?, updated_at = ? WHERE id = ?
	`, newName, now, initiative.ID); err != nil {
		return fmt.Errorf("rename initiative: %w", err)
	}
	if err := s.AppendHistoryEvent(ctx, task.HistoryEvent{
		InitiativeID: initiative.ID,
		EventType:    "rename",
		Property:     "name",
		OldValue:     oldName,
		NewValue:     newName,
		OccurredAt:   now,
	}); err != nil {
		return err
	}
	return nil
}

// SetTaskUrgency marks a task urgent and assigns high priority.
func (s *Store) SetTaskUrgency(ctx context.Context, taskID string, urgency float64) error {
	if math.IsNaN(urgency) || math.IsInf(urgency, 0) || urgency < 0 {
		return fmt.Errorf("invalid task urgency %v", urgency)
	}
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	now := timestamp()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET priority = 'H', urgency = ?, updated_at = ?
		WHERE id = ?
	`, urgency, now, current.ID); err != nil {
		return fmt.Errorf("set task urgency: %w", err)
	}
	if err := s.AppendHistoryEvent(ctx, task.HistoryEvent{
		TaskID:       current.ID,
		InitiativeID: current.InitiativeID,
		EventType:    "urgent",
		Property:     "urgency",
		OldValue:     fmt.Sprintf("%.1f", current.Urgency),
		NewValue:     fmt.Sprintf("%.1f", urgency),
		OccurredAt:   now,
	}); err != nil {
		return err
	}
	return nil
}

// SetTaskWait postpones readiness until the supplied normalized date.
func (s *Store) SetTaskWait(ctx context.Context, taskID string, waitUntil string) error {
	waitUntil = strings.TrimSpace(waitUntil)
	if waitUntil == "" {
		return errors.New("wait date is required")
	}
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	now := timestamp()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET wait_until = ?, updated_at = ? WHERE id = ?
	`, waitUntil, now, current.ID); err != nil {
		return fmt.Errorf("set task wait: %w", err)
	}
	if err := s.AppendHistoryEvent(ctx, task.HistoryEvent{
		TaskID:       current.ID,
		InitiativeID: current.InitiativeID,
		EventType:    "wait",
		Property:     "wait_until",
		OldValue:     current.WaitUntil,
		NewValue:     waitUntil,
		OccurredAt:   now,
	}); err != nil {
		return err
	}
	return nil
}

// AddDependency adds a dependency edge between tasks in the same project.
func (s *Store) AddDependency(ctx context.Context, taskID string, dependencyID string) error {
	current, dependency, err := s.dependencyTasks(ctx, taskID, dependencyID)
	if err != nil {
		return err
	}
	if current.ID == dependency.ID {
		return errors.New("a task cannot depend on itself")
	}
	if err := s.requireSameProject(ctx, current.ID, dependency.ID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO task_dependencies (task_id, depends_on_id)
		VALUES (?, ?)
	`, current.ID, dependency.ID); err != nil {
		return fmt.Errorf("add dependency: %w", err)
	}
	if err := s.RecordNativeHistory(ctx, current.ID, "block", "depends", "", dependency.ID, ""); err != nil {
		return err
	}
	return nil
}

// RemoveDependency removes one dependency edge between tasks.
func (s *Store) RemoveDependency(ctx context.Context, taskID string, dependencyID string) error {
	current, dependency, err := s.dependencyTasks(ctx, taskID, dependencyID)
	if err != nil {
		return err
	}
	if err := s.requireSameProject(ctx, current.ID, dependency.ID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM task_dependencies
		WHERE task_id = ? AND depends_on_id = ?
	`, current.ID, dependency.ID); err != nil {
		return fmt.Errorf("remove dependency: %w", err)
	}
	if err := s.RecordNativeHistory(ctx, current.ID, "unblock", "depends", dependency.ID, "", ""); err != nil {
		return err
	}
	return nil
}

func (s *Store) dependencyTasks(ctx context.Context, taskID string, dependencyID string) (task.Task, task.Task, error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return task.Task{}, task.Task{}, err
	}
	dependency, err := s.GetTask(ctx, dependencyID)
	if err != nil {
		return task.Task{}, task.Task{}, err
	}
	return current, dependency, nil
}

func (s *Store) requireSameProject(ctx context.Context, firstID string, secondID string) error {
	firstProject, err := s.taskProjectID(ctx, firstID)
	if err != nil {
		return err
	}
	secondProject, err := s.taskProjectID(ctx, secondID)
	if err != nil {
		return err
	}
	if firstProject != secondProject {
		return fmt.Errorf("tasks %s and %s belong to different projects", firstID[:8], secondID[:8])
	}
	return nil
}

func (s *Store) taskProjectID(ctx context.Context, taskID string) (string, error) {
	var projectID string
	err := s.db.QueryRowContext(ctx, `
		SELECT i.project_id
		FROM tasks t
		JOIN initiatives i ON i.id = t.initiative_id
		WHERE t.id = ?
	`, taskID).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("task %q not found", taskID)
	}
	if err != nil {
		return "", fmt.Errorf("read task project: %w", err)
	}
	return projectID, nil
}
