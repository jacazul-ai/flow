package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/task"
)

// GetSessionNote returns a persisted session note and whether it exists.
func (s *Store) GetSessionNote(ctx context.Context, projectID string, sessionID string) (task.SessionNote, bool, error) {
	if projectID == "" || sessionID == "" {
		return task.SessionNote{}, false, errors.New("project ID and session ID are required")
	}
	var note task.SessionNote
	err := s.db.QueryRowContext(ctx, `
		SELECT project_id, session_id, content, acknowledged_at, updated_at
		FROM session_notes
		WHERE project_id = ? AND session_id = ?
	`, projectID, sessionID).Scan(
		&note.ProjectID,
		&note.SessionID,
		&note.Content,
		&note.AcknowledgedAt,
		&note.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return task.SessionNote{}, false, nil
	}
	if err != nil {
		return task.SessionNote{}, false, fmt.Errorf("get session note: %w", err)
	}
	return note, true, nil
}

// SaveSessionNote persists a session handoff note.
func (s *Store) SaveSessionNote(ctx context.Context, note task.SessionNote) error {
	if note.ProjectID == "" || note.SessionID == "" {
		return errors.New("project ID and session ID are required")
	}
	if strings.TrimSpace(note.Content) == "" {
		return errors.New("session note content is required")
	}
	now := timestamp()
	if note.UpdatedAt == "" {
		note.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_notes
			(project_id, session_id, content, acknowledged_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (project_id, session_id) DO UPDATE SET
			content = excluded.content,
			acknowledged_at = excluded.acknowledged_at,
			updated_at = excluded.updated_at
	`, note.ProjectID, note.SessionID, note.Content, note.AcknowledgedAt, note.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save session note: %w", err)
	}
	return nil
}

// AcknowledgeSessionNote marks a session note as read.
func (s *Store) AcknowledgeSessionNote(ctx context.Context, projectID string, sessionID string) (task.SessionNote, bool, error) {
	note, found, err := s.GetSessionNote(ctx, projectID, sessionID)
	if err != nil || !found || note.AcknowledgedAt != "" {
		return note, found, err
	}

	note.AcknowledgedAt = timestamp()
	note.Content = strings.TrimRight(note.Content, "\n") +
		"\nacknowledged: " + note.AcknowledgedAt + "\n"
	note.UpdatedAt = timestamp()
	if err := s.SaveSessionNote(ctx, note); err != nil {
		return task.SessionNote{}, false, err
	}
	return note, true, nil
}

// DeleteSession removes one non-global session and its derived state.
func (s *Store) DeleteSession(ctx context.Context, projectID string, sessionID string) error {
	if projectID == "" || sessionID == "" {
		return errors.New("project ID and session ID are required")
	}
	if sessionID == "global" {
		return errors.New("global session cannot be deleted")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session deletion: %w", err)
	}
	defer tx.Rollback()
	for _, statement := range []string{
		"DELETE FROM session_notes WHERE project_id = ? AND session_id = ?",
		"DELETE FROM cache_entries WHERE project_id = ? AND session_id = ?",
		"DELETE FROM sessions WHERE project_id = ? AND session_id = ?",
	} {
		if _, err := tx.ExecContext(ctx, statement, projectID, sessionID); err != nil {
			return fmt.Errorf("delete session state: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session deletion: %w", err)
	}
	return nil
}
