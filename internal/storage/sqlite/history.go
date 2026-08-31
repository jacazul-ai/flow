package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jacazul-ai/jaflow/internal/task"
)

// AppendHistoryEvent appends one immutable task history event.
func (s *Store) AppendHistoryEvent(ctx context.Context, event task.HistoryEvent) error {
	if strings.TrimSpace(event.TaskID) == "" && strings.TrimSpace(event.InitiativeID) == "" {
		return errors.New("history task ID or initiative ID is required")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return errors.New("history event type is required")
	}
	if strings.TrimSpace(event.OccurredAt) == "" {
		return errors.New("history event timestamp is required")
	}
	if event.TaskID != "" {
		current, err := s.GetTask(ctx, event.TaskID)
		if err != nil {
			return err
		}
		if event.InitiativeID == "" {
			event.InitiativeID = current.InitiativeID
		} else if event.InitiativeID != current.InitiativeID {
			return fmt.Errorf("history task %s crosses initiative boundary", current.ID[:8])
		}
	} else if err := s.requireInitiativeID(ctx, event.InitiativeID); err != nil {
		return err
	}
	if event.ID == "" {
		event.ID = newUUID()
	}
	if event.Source == "" {
		event.Source = "native"
	}
	if event.SourceEventID == "" {
		event.SourceEventID = event.ID
	}
	if event.RecordedAt == "" {
		event.RecordedAt = timestamp()
	}
	return appendHistoryEvent(ctx, s.db, event)
}

// RecordNativeHistory records one native mutation for a task.
func (s *Store) RecordNativeHistory(ctx context.Context, taskID string, eventType string, property string, oldValue string, newValue string, sessionID string) error {
	return s.AppendHistoryEvent(ctx, task.HistoryEvent{
		TaskID:     taskID,
		EventType:  eventType,
		Property:   property,
		OldValue:   oldValue,
		NewValue:   newValue,
		OccurredAt: timestamp(),
		SessionID:  sessionID,
	})
}

// ListHistory returns one task's immutable events in source order.
func (s *Store) ListHistory(ctx context.Context, taskID string) ([]task.HistoryEvent, error) {
	current, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, initiative_id, source, source_event_id, sequence,
		       event_type, property, old_value, new_value, occurred_at, actor,
		       session_id, recorded_at
		FROM workflow_history
		WHERE task_id = ?
		ORDER BY occurred_at, sequence, id
	`, current.ID)
	if err != nil {
		return nil, fmt.Errorf("list task history: %w", err)
	}
	defer rows.Close()
	return scanHistoryRows(rows)
}

// ListInitiativeHistory returns events for an initiative and its tasks.
func (s *Store) ListInitiativeHistory(ctx context.Context, projectID string, initiativeName string) ([]task.HistoryEvent, error) {
	initiative, err := s.FindInitiative(ctx, projectID, initiativeName)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, initiative_id, source, source_event_id, sequence,
		       event_type, property, old_value, new_value, occurred_at, actor,
		       session_id, recorded_at
		FROM workflow_history
		WHERE initiative_id = ?
		ORDER BY occurred_at, sequence, id
	`, initiative.ID)
	if err != nil {
		return nil, fmt.Errorf("list initiative history: %w", err)
	}
	defer rows.Close()
	return scanHistoryRows(rows)
}

func scanHistoryRows(rows *sql.Rows) ([]task.HistoryEvent, error) {
	var events []task.HistoryEvent
	for rows.Next() {
		var event task.HistoryEvent
		var taskID sql.NullString
		var initiativeID sql.NullString
		if err := rows.Scan(
			&event.ID,
			&taskID,
			&initiativeID,
			&event.Source,
			&event.SourceEventID,
			&event.Sequence,
			&event.EventType,
			&event.Property,
			&event.OldValue,
			&event.NewValue,
			&event.OccurredAt,
			&event.Actor,
			&event.SessionID,
			&event.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("scan history event: %w", err)
		}
		event.TaskID = taskID.String
		event.InitiativeID = initiativeID.String
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history events: %w", err)
	}
	return events, nil
}

func (s *Store) requireInitiativeID(ctx context.Context, initiativeID string) error {
	var exists int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM initiatives WHERE id = ?", initiativeID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("initiative %q not found", initiativeID)
	}
	if err != nil {
		return fmt.Errorf("read initiative: %w", err)
	}
	return nil
}

type historyExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func appendHistoryEvent(ctx context.Context, executor historyExecutor, event task.HistoryEvent) error {
	if event.ID == "" {
		event.ID = newUUID()
	}
	if event.Source == "" {
		event.Source = "native"
	}
	if event.SourceEventID == "" {
		event.SourceEventID = event.ID
	}
	if event.RecordedAt == "" {
		event.RecordedAt = timestamp()
	}
	_, err := executor.ExecContext(ctx, `
		INSERT INTO workflow_history
			(id, task_id, initiative_id, source, source_event_id, sequence,
			 event_type, property, old_value, new_value, occurred_at, actor,
			 session_id, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source, source_event_id) DO NOTHING
	`, event.ID, nullableValue(event.TaskID), nullableValue(event.InitiativeID),
		event.Source, event.SourceEventID, event.Sequence, event.EventType,
		event.Property, event.OldValue, event.NewValue, event.OccurredAt,
		event.Actor, event.SessionID, event.RecordedAt)
	if err != nil {
		return fmt.Errorf("append history event: %w", err)
	}
	return nil
}

func nullableValue(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func historyEventExists(ctx context.Context, tx *sql.Tx, source string, sourceEventID string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM workflow_history
		WHERE source = ? AND source_event_id = ?
	`, source, sourceEventID).Scan(&count); err != nil {
		return false, fmt.Errorf("check history event: %w", err)
	}
	return count > 0, nil
}
