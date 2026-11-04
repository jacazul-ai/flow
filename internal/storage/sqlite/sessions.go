package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jacazul-ai/flow/internal/task"
)

func (s *Store) ensureSessionActive(ctx context.Context, projectID string, sessionID string) error {
	var state string
	err := s.db.QueryRowContext(ctx, `
		SELECT state FROM sessions WHERE project_id = ? AND session_id = ?
	`, projectID, sessionID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check session state: %w", err)
	}
	if state != "purged" {
		return nil
	}
	return purgedSessionError(sessionID)
}

func purgedSessionError(sessionID string) error {
	return fmt.Errorf("session %s is purged\nACTION: Use a new session ID; purged sessions cannot be reactivated.", sessionID)
}

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

// PurgeSession marks one orphan session as logically purged.
func (s *Store) PurgeSession(ctx context.Context, projectID string, sessionID string, currentSessionID string) error {
	if projectID == "" || sessionID == "" {
		return errors.New("project ID and session ID are required")
	}
	if sessionID == "global" || sessionID == currentSessionID {
		return fmt.Errorf("session %s cannot be purged", sessionID)
	}
	now := timestamp()
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET state = 'purged', purged_at = ?, updated_at = ?
		WHERE project_id = ? AND session_id = ? AND state != 'purged'
	`, now, now, projectID, sessionID)
	if err != nil {
		return fmt.Errorf("purge session: %w", err)
	}
	return nil
}

// HardDeletePurgedSession physically removes one already-purged session.
func (s *Store) HardDeletePurgedSession(ctx context.Context, projectID string, sessionID string, currentSessionID string) error {
	if projectID == "" || sessionID == "" {
		return errors.New("project ID and session ID are required")
	}
	if sessionID == "global" || sessionID == currentSessionID {
		return fmt.Errorf("session %s cannot be hard-purged", sessionID)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin hard session purge: %w", err)
	}
	defer tx.Rollback()
	var state string
	if err := tx.QueryRowContext(ctx, `
		SELECT state FROM sessions WHERE project_id = ? AND session_id = ?
	`, projectID, sessionID).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("session %s not found", sessionID)
		}
		return fmt.Errorf("check session purge state: %w", err)
	}
	if state != "purged" {
		return fmt.Errorf("session %s is not logically purged\nACTION: Run session purge first.", sessionID)
	}
	for _, statement := range []string{
		"DELETE FROM session_dumps WHERE project_id = ? AND session_id = ?",
		"DELETE FROM cache_entries WHERE project_id = ? AND session_id = ?",
		"DELETE FROM sessions WHERE project_id = ? AND session_id = ?",
	} {
		if _, err := tx.ExecContext(ctx, statement, projectID, sessionID); err != nil {
			return fmt.Errorf("hard-purge session state: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit hard session purge: %w", err)
	}
	return nil
}

// ExitSession moves one non-global session from its independent focus back to
// the global focus: the row stays with scope global and an empty focus, and
// the derived cache is dropped. Like removing the legacy focus file, it keeps
// the session's handoff notes and dumps; only session purge --hard deletes
// history.
func (s *Store) ExitSession(ctx context.Context, projectID string, sessionID string) error {
	if projectID == "" || sessionID == "" {
		return errors.New("project ID and session ID are required")
	}
	if sessionID == "global" {
		return errors.New("global session cannot be exited")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session exit: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM cache_entries WHERE project_id = ? AND session_id = ?
	`, projectID, sessionID); err != nil {
		return fmt.Errorf("exit session cache: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET scope = ?, focused_initiative_id = '', focused_task_id = '',
		    task_stack_json = '[]', plans_of_interest_json = '[]', updated_at = ?
		WHERE project_id = ? AND session_id = ?
	`, task.ScopeGlobal, timestamp(), projectID, sessionID); err != nil {
		return fmt.Errorf("exit session focus: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session exit: %w", err)
	}
	return nil
}

// FocusScope resolves the focus a session writes: its own session ID when it
// is in an independent focus, else the project's shared "global" focus. A
// purged session never acts again, so resolving its scope fails.
func (s *Store) FocusScope(ctx context.Context, projectID string, sessionID string) (string, error) {
	if projectID == "" || sessionID == "" {
		return "", errors.New("project ID and session ID are required")
	}
	if sessionID == "global" {
		return "global", nil
	}
	var scope string
	var state string
	err := s.db.QueryRowContext(ctx, `
		SELECT scope, state FROM sessions WHERE project_id = ? AND session_id = ?
	`, projectID, sessionID).Scan(&scope, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return "global", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve focus scope: %w", err)
	}
	if state == "purged" {
		return "", purgedSessionError(sessionID)
	}
	if scope == task.ScopeIndependent {
		return sessionID, nil
	}
	return "global", nil
}

// TouchSession records that a session is active. A session without a row
// joins the global focus with an empty presence row; an existing row only
// refreshes its activity time.
func (s *Store) TouchSession(ctx context.Context, projectID string, sessionID string) error {
	if projectID == "" || sessionID == "" {
		return errors.New("project ID and session ID are required")
	}
	if err := s.ensureSessionActive(ctx, projectID, sessionID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions
			(project_id, session_id, focused_initiative_id, focused_task_id,
			 task_stack_json, plans_of_interest_json, updated_at, scope)
		VALUES (?, ?, '', '', '[]', '[]', ?, ?)
		ON CONFLICT (project_id, session_id) DO UPDATE SET
			updated_at = excluded.updated_at
	`, projectID, sessionID, timestamp(), task.ScopeGlobal)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

// scopeForSession returns the scope a focus row written under sessionID has:
// the project's global row is global, any other written row is independent.
func scopeForSession(sessionID string) string {
	if sessionID == "global" {
		return task.ScopeGlobal
	}
	return task.ScopeIndependent
}

// scopeLabel maps a resolved focus scope to the handoff scope label.
func scopeLabel(focusScope string) string {
	if focusScope == "global" {
		return task.ScopeGlobal
	}
	return task.ScopeIndependent
}

// HandoffScope resolves the scope of a session's handoff: global while the
// session reads the global focus, ind while it has its own.
func (s *Store) HandoffScope(ctx context.Context, projectID string, sessionID string) (string, error) {
	focusScope, err := s.FocusScope(ctx, projectID, sessionID)
	if err != nil {
		return "", err
	}
	return scopeLabel(focusScope), nil
}

// ClearGlobalScopeCache drops the derived cache of every session of one
// project that reads the global focus: the global scope itself and each
// session not in an independent focus.
func (s *Store) ClearGlobalScopeCache(ctx context.Context, projectID string) error {
	if projectID == "" {
		return errors.New("project ID is required")
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM cache_entries
		WHERE project_id = ? AND session_id NOT IN (
			SELECT session_id FROM sessions
			WHERE project_id = ? AND scope = 'ind' AND session_id != 'global'
		)
	`, projectID, projectID)
	if err != nil {
		return fmt.Errorf("clear global scope cache: %w", err)
	}
	return nil
}
