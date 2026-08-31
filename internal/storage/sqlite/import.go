package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jacazul-ai/jaflow/internal/task"
)

// ApplyImport persists a validated import bundle in one project transaction.
func (s *Store) ApplyImport(ctx context.Context, bundle task.ImportBundle) (task.ImportResult, error) {
	if err := validateImportProject(bundle); err != nil {
		return task.ImportResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return task.ImportResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return task.ImportResult{}, fmt.Errorf("begin import transaction: %w", err)
	}
	defer tx.Rollback()

	result := task.ImportResult{}
	for _, initiative := range bundle.Initiatives {
		exists, err := initiativeExists(ctx, tx, initiative.ID, bundle.ProjectID)
		if err != nil {
			return task.ImportResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO initiatives
				(id, project_id, name, status, external_ticket, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
				project_id = excluded.project_id,
				name = excluded.name,
				status = excluded.status,
				external_ticket = excluded.external_ticket,
				updated_at = excluded.updated_at
		`, initiative.ID, initiative.ProjectID, initiative.Name, initiative.Status,
			initiative.ExternalTicket, initiative.CreatedAt, initiative.UpdatedAt); err != nil {
			return task.ImportResult{}, fmt.Errorf("import initiative %s: %w", initiative.ID, err)
		}
		if !exists {
			result.Created++
		} else {
			result.Updated++
		}
	}

	for _, imported := range bundle.Tasks {
		exists, err := taskExists(ctx, tx, imported.ID, bundle.ProjectID)
		if err != nil {
			return task.ImportResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tasks
				(id, initiative_id, description, mode, status, outcome,
				 external_ticket, started_at, completed_at, disposition, due_at,
				 priority, urgency, wait_until, task_mode_code, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
				initiative_id = excluded.initiative_id,
				description = excluded.description,
				mode = excluded.mode,
				status = excluded.status,
				outcome = excluded.outcome,
				external_ticket = excluded.external_ticket,
				started_at = excluded.started_at,
				completed_at = excluded.completed_at,
				disposition = excluded.disposition,
				due_at = excluded.due_at,
				priority = excluded.priority,
				urgency = excluded.urgency,
				wait_until = excluded.wait_until,
				task_mode_code = excluded.task_mode_code,
				updated_at = excluded.updated_at
		`, imported.ID, imported.InitiativeID, imported.Description,
			legacyModeName(imported.Mode), imported.Status, imported.Outcome,
			imported.ExternalTicket, imported.StartedAt, imported.CompletedAt,
			imported.Disposition, imported.DueAt, imported.Priority, imported.Urgency,
			imported.WaitUntil, imported.Mode, imported.CreatedAt, imported.UpdatedAt); err != nil {
			return task.ImportResult{}, fmt.Errorf("import task %s: %w", imported.ID, err)
		}
		if !exists {
			result.Created++
		} else {
			result.Updated++
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM task_dependencies WHERE task_id = ?`, imported.ID); err != nil {
			return task.ImportResult{}, fmt.Errorf("replace task dependencies %s: %w", imported.ID, err)
		}
	}

	for _, dependency := range bundle.Dependencies {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_dependencies (task_id, depends_on_id)
			VALUES (?, ?)
		`, dependency.TaskID, dependency.DependsOnID); err != nil {
			return task.ImportResult{}, fmt.Errorf("import dependency %s: %w", dependency.TaskID, err)
		}
		result.Dependencies++
	}

	for _, annotation := range bundle.Annotations {
		var exists int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM annotations
			WHERE task_id = ? AND kind = ? AND body = ? AND created_at = ?
		`, annotation.TaskID, annotation.Kind, annotation.Body, annotation.CreatedAt).Scan(&exists); err != nil {
			return task.ImportResult{}, fmt.Errorf("check annotation %s: %w", annotation.TaskID, err)
		}
		if exists > 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO annotations (task_id, kind, body, created_at)
			VALUES (?, ?, ?, ?)
		`, annotation.TaskID, annotation.Kind, annotation.Body, annotation.CreatedAt); err != nil {
			return task.ImportResult{}, fmt.Errorf("import annotation %s: %w", annotation.TaskID, err)
		}
		result.Annotations++
	}

	for _, event := range bundle.History {
		if event.TaskID == "" && event.InitiativeID == "" {
			return task.ImportResult{}, errors.New("history event requires task or initiative ID")
		}
		if event.Source == "" || event.SourceEventID == "" || event.EventType == "" || event.OccurredAt == "" {
			return task.ImportResult{}, fmt.Errorf("history event %q is incomplete", event.ID)
		}
		if event.TaskID != "" {
			exists, err := taskExists(ctx, tx, event.TaskID, bundle.ProjectID)
			if err != nil {
				return task.ImportResult{}, err
			}
			if !exists {
				return task.ImportResult{}, fmt.Errorf("history task %s not found", event.TaskID)
			}
		}
		if event.InitiativeID != "" {
			exists, err := initiativeExists(ctx, tx, event.InitiativeID, bundle.ProjectID)
			if err != nil {
				return task.ImportResult{}, err
			}
			if !exists {
				return task.ImportResult{}, fmt.Errorf("history initiative %s not found", event.InitiativeID)
			}
		}
		initiativeID := event.InitiativeID
		if initiativeID == "" {
			var err error
			initiativeID, err = taskInitiativeID(ctx, tx, event.TaskID)
			if err != nil {
				return task.ImportResult{}, err
			}
		}
		eventID := event.ID
		if eventID == "" {
			eventID = newUUID()
		}
		recordedAt := event.RecordedAt
		if recordedAt == "" {
			recordedAt = timestamp()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO workflow_history
				(id, task_id, initiative_id, source, source_event_id, sequence,
				 event_type, property, old_value, new_value, occurred_at, actor,
				 session_id, recorded_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (source, source_event_id) DO NOTHING
		`, eventID, nullableValue(event.TaskID), nullableValue(initiativeID),
			event.Source, event.SourceEventID, event.Sequence, event.EventType,
			event.Property, event.OldValue, event.NewValue, event.OccurredAt,
			event.Actor, event.SessionID, recordedAt); err != nil {
			return task.ImportResult{}, fmt.Errorf("import history event %s: %w", event.SourceEventID, err)
		}
		result.History++
	}

	for _, session := range bundle.Sessions {
		if session.State.ProjectID != bundle.ProjectID || session.State.SessionID == "" {
			return task.ImportResult{}, fmt.Errorf("session %q crosses import project boundary", session.State.SessionID)
		}
		stackJSON, err := json.Marshal(session.State.TaskStack)
		if err != nil {
			return task.ImportResult{}, fmt.Errorf("encode imported session %s stack: %w", session.State.SessionID, err)
		}
		interestsJSON, err := json.Marshal(session.State.PlansOfInterest)
		if err != nil {
			return task.ImportResult{}, fmt.Errorf("encode imported session %s interests: %w", session.State.SessionID, err)
		}
		updatedAt := session.UpdatedAt
		if updatedAt == "" {
			updatedAt = timestamp()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO sessions
				(project_id, session_id, focused_initiative_id, focused_task_id,
				 task_stack_json, plans_of_interest_json, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (project_id, session_id) DO UPDATE SET
				focused_initiative_id = excluded.focused_initiative_id,
				focused_task_id = excluded.focused_task_id,
				task_stack_json = excluded.task_stack_json,
				plans_of_interest_json = excluded.plans_of_interest_json,
				updated_at = excluded.updated_at
		`, bundle.ProjectID, session.State.SessionID, session.State.InitiativeID,
			session.State.FocusedTaskID, string(stackJSON), string(interestsJSON), updatedAt); err != nil {
			return task.ImportResult{}, fmt.Errorf("import session %s: %w", session.State.SessionID, err)
		}
	}

	for _, note := range bundle.SessionNotes {
		if note.ProjectID != bundle.ProjectID || note.SessionID == "" {
			return task.ImportResult{}, fmt.Errorf("session note %q crosses import project boundary", note.SessionID)
		}
		updatedAt := note.UpdatedAt
		if updatedAt == "" {
			updatedAt = timestamp()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO session_notes
				(project_id, session_id, content, acknowledged_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (project_id, session_id) DO UPDATE SET
				content = excluded.content,
				acknowledged_at = excluded.acknowledged_at,
				updated_at = excluded.updated_at
		`, note.ProjectID, note.SessionID, note.Content, note.AcknowledgedAt, updatedAt); err != nil {
			return task.ImportResult{}, fmt.Errorf("import session note %s: %w", note.SessionID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM cache_entries WHERE project_id = ?`, bundle.ProjectID); err != nil {
		return task.ImportResult{}, fmt.Errorf("clear imported project cache: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return task.ImportResult{}, fmt.Errorf("commit import: %w", err)
	}
	return result, nil
}

func validateImportProject(bundle task.ImportBundle) error {
	if strings.TrimSpace(bundle.ProjectID) == "" {
		return errors.New("import project ID is required")
	}
	for _, initiative := range bundle.Initiatives {
		if initiative.ProjectID != bundle.ProjectID {
			return fmt.Errorf("initiative %s crosses project boundary", initiative.ID)
		}
	}
	for _, current := range bundle.Tasks {
		if current.ID == "" || current.InitiativeID == "" || current.Description == "" {
			return errors.New("import task requires ID, initiative, and description")
		}
		if !current.Mode.Valid() {
			return fmt.Errorf("task %s has invalid mode %d", current.ID, current.Mode)
		}
		if current.Priority != "L" && current.Priority != "M" && current.Priority != "H" {
			return fmt.Errorf("task %s has invalid priority %q", current.ID, current.Priority)
		}
	}
	for _, dependency := range bundle.Dependencies {
		if dependency.TaskID == dependency.DependsOnID {
			return fmt.Errorf("task %s depends on itself", dependency.TaskID)
		}
	}
	for _, event := range bundle.History {
		if event.TaskID == "" && event.InitiativeID == "" {
			return errors.New("history event requires task or initiative ID")
		}
		if event.Source == "" || event.SourceEventID == "" || event.EventType == "" || event.OccurredAt == "" {
			return fmt.Errorf("history event %q is incomplete", event.ID)
		}
	}
	return nil
}

func initiativeExists(ctx context.Context, tx *sql.Tx, initiativeID string, projectID string) (bool, error) {
	var existingProject string
	err := tx.QueryRowContext(ctx, `SELECT project_id FROM initiatives WHERE id = ?`, initiativeID).Scan(&existingProject)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check imported initiative %s: %w", initiativeID, err)
	}
	if existingProject != projectID {
		return false, fmt.Errorf("initiative %s belongs to another project", initiativeID)
	}
	return true, nil
}

func taskInitiativeID(ctx context.Context, tx *sql.Tx, taskID string) (string, error) {
	var initiativeID string
	err := tx.QueryRowContext(ctx, "SELECT initiative_id FROM tasks WHERE id = ?", taskID).Scan(&initiativeID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("task %q not found", taskID)
	}
	if err != nil {
		return "", fmt.Errorf("read task initiative: %w", err)
	}
	return initiativeID, nil
}

func taskExists(ctx context.Context, tx *sql.Tx, taskID string, projectID string) (bool, error) {
	var existingProject string
	err := tx.QueryRowContext(ctx, `
		SELECT i.project_id
		FROM tasks t
		JOIN initiatives i ON i.id = t.initiative_id
		WHERE t.id = ?
	`, taskID).Scan(&existingProject)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check imported task %s: %w", taskID, err)
	}
	if existingProject != projectID {
		return false, fmt.Errorf("task %s belongs to another project", taskID)
	}
	return true, nil
}
