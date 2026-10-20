package sqlite

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jacazul-ai/flow/internal/task"
)

const sessionDumpEncoding = "gzip"

// CreateSessionDumpDraft creates the next immutable-history draft for a session.
func (s *Store) CreateSessionDumpDraft(ctx context.Context, projectID string, sessionID string, focusSummary string, content string) (task.SessionDump, error) {
	if projectID == "" || sessionID == "" {
		return task.SessionDump{}, errors.New("project ID and session ID are required")
	}
	if strings.TrimSpace(content) == "" {
		return task.SessionDump{}, errors.New("session dump content is required")
	}
	latest, found, err := s.GetLatestSessionDump(ctx, projectID, sessionID)
	if err != nil {
		return task.SessionDump{}, err
	}
	if found && latest.Status != task.SessionDumpRead {
		return task.SessionDump{}, fmt.Errorf("session dump is already pending for session %s\nACTION: Read and acknowledge the pending dump before creating another.", sessionID)
	}

	blob, size, checksum, err := encodeSessionDump(content)
	if err != nil {
		return task.SessionDump{}, err
	}
	var sequence int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1
		FROM session_dumps
		WHERE project_id = ? AND session_id = ?
	`, projectID, sessionID).Scan(&sequence); err != nil {
		return task.SessionDump{}, fmt.Errorf("allocate session dump sequence: %w", err)
	}
	now := timestamp()
	dump := task.SessionDump{
		ID:              newUUID(),
		ProjectID:       projectID,
		SessionID:       sessionID,
		Sequence:        sequence,
		Status:          task.SessionDumpDraft,
		CreatedAt:       now,
		FocusSummary:    focusSummary,
		ContentEncoding: sessionDumpEncoding,
		Content:         []byte(content),
		ContentSize:     size,
		ContentSHA256:   checksum,
		UpdatedAt:       now,
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO session_dumps
			(id, project_id, session_id, sequence, status, created_at,
			 delivered_at, acknowledged_at, focus_summary, content_encoding,
			 content_blob, content_size, content_sha256, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '', '', ?, ?, ?, ?, ?, ?)
	`, dump.ID, dump.ProjectID, dump.SessionID, dump.Sequence, dump.Status,
		dump.CreatedAt, dump.FocusSummary, dump.ContentEncoding, blob,
		dump.ContentSize, dump.ContentSHA256, dump.UpdatedAt)
	if err != nil {
		return task.SessionDump{}, fmt.Errorf("create session dump draft: %w", err)
	}
	return dump, nil
}

// FinalizeSessionDump seals a draft as the next pending dump.
func (s *Store) FinalizeSessionDump(ctx context.Context, projectID string, sessionID string, dumpID string, content string) (task.SessionDump, error) {
	dump, found, err := s.GetSessionDump(ctx, projectID, sessionID, dumpID)
	if err != nil {
		return task.SessionDump{}, err
	}
	if !found {
		return task.SessionDump{}, fmt.Errorf("session dump %q not found", dumpID)
	}
	if dump.Status != task.SessionDumpDraft {
		return task.SessionDump{}, fmt.Errorf("session dump %s is not a draft", shortSessionDumpID(dump.ID))
	}
	if strings.TrimSpace(content) == "" {
		return task.SessionDump{}, errors.New("finalized session dump content is required")
	}
	blob, size, checksum, err := encodeSessionDump(content)
	if err != nil {
		return task.SessionDump{}, err
	}
	now := timestamp()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE session_dumps
		SET status = ?, content_blob = ?, content_size = ?, content_sha256 = ?, updated_at = ?
		WHERE id = ? AND project_id = ? AND session_id = ? AND status = ?
	`, task.SessionDumpPending, blob, size, checksum, now, dump.ID, projectID,
		sessionID, task.SessionDumpDraft); err != nil {
		return task.SessionDump{}, fmt.Errorf("finalize session dump: %w", err)
	}
	dump.Status = task.SessionDumpPending
	dump.Content = []byte(content)
	dump.ContentSize = size
	dump.ContentSHA256 = checksum
	dump.UpdatedAt = now
	return dump, nil
}

// GetLatestSessionDump returns the latest dump and its decoded content.
func (s *Store) GetLatestSessionDump(ctx context.Context, projectID string, sessionID string) (task.SessionDump, bool, error) {
	return s.getSessionDump(ctx, `
		SELECT id, project_id, session_id, sequence, status, created_at,
		       delivered_at, acknowledged_at, focus_summary, content_encoding,
		       content_blob, content_size, content_sha256, updated_at
		FROM session_dumps
		WHERE project_id = ? AND session_id = ?
		ORDER BY sequence DESC
		LIMIT 1
	`, projectID, sessionID)
}

// GetLatestPendingSessionDump returns the current pending dump, if any.
func (s *Store) GetLatestPendingSessionDump(ctx context.Context, projectID string, sessionID string) (task.SessionDump, bool, error) {
	return s.getSessionDump(ctx, `
		SELECT id, project_id, session_id, sequence, status, created_at,
		       delivered_at, acknowledged_at, focus_summary, content_encoding,
		       content_blob, content_size, content_sha256, updated_at
		FROM session_dumps
		WHERE project_id = ? AND session_id = ? AND status = ?
		ORDER BY sequence DESC
		LIMIT 1
	`, projectID, sessionID, task.SessionDumpPending)
}

// GetSessionDump returns one dump and its decoded content.
func (s *Store) GetSessionDump(ctx context.Context, projectID string, sessionID string, dumpID string) (task.SessionDump, bool, error) {
	resolvedID, err := s.resolveSessionDumpID(ctx, projectID, sessionID, dumpID)
	if err != nil {
		return task.SessionDump{}, false, err
	}
	return s.getSessionDump(ctx, `
		SELECT id, project_id, session_id, sequence, status, created_at,
		       delivered_at, acknowledged_at, focus_summary, content_encoding,
		       content_blob, content_size, content_sha256, updated_at
		FROM session_dumps
		WHERE project_id = ? AND session_id = ? AND id = ?
	`, projectID, sessionID, resolvedID)
}

func (s *Store) resolveSessionDumpID(ctx context.Context, projectID string, sessionID string, dumpID string) (string, error) {
	dumpID = strings.TrimSpace(dumpID)
	if len(dumpID) >= 36 || !isHexDumpID(dumpID) {
		return dumpID, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM session_dumps
		WHERE project_id = ? AND session_id = ? AND id LIKE ?
		ORDER BY id
		LIMIT 2
	`, projectID, sessionID, dumpID+"%")
	if err != nil {
		return "", fmt.Errorf("resolve session dump %q: %w", dumpID, err)
	}
	defer rows.Close()

	var matches []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", fmt.Errorf("scan session dump reference: %w", err)
		}
		matches = append(matches, id)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate session dump references: %w", err)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("session dump reference %q is ambiguous\nACTION: Use the full dump UUID.", dumpID)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return dumpID, nil
}

func isHexDumpID(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

// DeliverLatestSessionDump marks and returns the latest pending dump.
func (s *Store) DeliverLatestSessionDump(ctx context.Context, projectID string, sessionID string) (task.SessionDump, bool, error) {
	dump, found, err := s.GetLatestPendingSessionDump(ctx, projectID, sessionID)
	if err != nil || !found {
		return dump, found, err
	}
	now := timestamp()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE session_dumps SET delivered_at = ?, updated_at = ?
		WHERE id = ? AND project_id = ? AND session_id = ? AND status = ?
	`, now, now, dump.ID, projectID, sessionID, task.SessionDumpPending); err != nil {
		return task.SessionDump{}, false, fmt.Errorf("mark session dump delivered: %w", err)
	}
	dump.DeliveredAt = now
	dump.UpdatedAt = now
	return dump, true, nil
}

// AcknowledgeLatestSessionDump marks the latest pending dump as read.
func (s *Store) AcknowledgeLatestSessionDump(ctx context.Context, projectID string, sessionID string) (task.SessionDump, bool, error) {
	dump, found, err := s.GetLatestPendingSessionDump(ctx, projectID, sessionID)
	if err != nil || !found {
		return dump, found, err
	}
	now := timestamp()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE session_dumps
		SET status = ?, acknowledged_at = ?, updated_at = ?
		WHERE id = ? AND project_id = ? AND session_id = ? AND status = ?
	`, task.SessionDumpRead, now, now, dump.ID, projectID, sessionID, task.SessionDumpPending); err != nil {
		return task.SessionDump{}, false, fmt.Errorf("acknowledge session dump: %w", err)
	}
	dump.Status = task.SessionDumpRead
	dump.AcknowledgedAt = now
	dump.UpdatedAt = now
	return dump, true, nil
}

// ListSessionDumps returns dump metadata in sequence order without payloads.
func (s *Store) ListSessionDumps(ctx context.Context, projectID string, sessionID string) ([]task.SessionDump, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, session_id, sequence, status, created_at,
		       delivered_at, acknowledged_at, focus_summary, content_encoding,
		       content_size, content_sha256, updated_at
		FROM session_dumps
		WHERE project_id = ? AND session_id = ?
		ORDER BY sequence
	`, projectID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session dumps: %w", err)
	}
	defer rows.Close()
	var dumps []task.SessionDump
	for rows.Next() {
		var dump task.SessionDump
		if err := rows.Scan(
			&dump.ID, &dump.ProjectID, &dump.SessionID, &dump.Sequence,
			&dump.Status, &dump.CreatedAt, &dump.DeliveredAt,
			&dump.AcknowledgedAt, &dump.FocusSummary, &dump.ContentEncoding,
			&dump.ContentSize, &dump.ContentSHA256, &dump.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session dump: %w", err)
		}
		dumps = append(dumps, dump)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session dumps: %w", err)
	}
	return dumps, nil
}

func (s *Store) getSessionDump(ctx context.Context, query string, args ...any) (task.SessionDump, bool, error) {
	var dump task.SessionDump
	var status string
	var blob []byte
	var encoding string
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&dump.ID, &dump.ProjectID, &dump.SessionID, &dump.Sequence,
		&status, &dump.CreatedAt, &dump.DeliveredAt, &dump.AcknowledgedAt,
		&dump.FocusSummary, &encoding, &blob, &dump.ContentSize,
		&dump.ContentSHA256, &dump.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return task.SessionDump{}, false, nil
	}
	if err != nil {
		return task.SessionDump{}, false, fmt.Errorf("get session dump: %w", err)
	}
	content, err := decodeSessionDump(encoding, blob)
	if err != nil {
		return task.SessionDump{}, false, fmt.Errorf("decode session dump %s: %w", dump.ID, err)
	}
	checksum := sha256.Sum256([]byte(content))
	if fmt.Sprintf("%x", checksum) != dump.ContentSHA256 {
		return task.SessionDump{}, false, fmt.Errorf("session dump %s content hash mismatch", dump.ID)
	}
	dump.Status = task.SessionDumpStatus(status)
	dump.ContentEncoding = encoding
	dump.Content = []byte(content)
	return dump, true, nil
}

func encodeSessionDump(content string) ([]byte, int64, string, error) {
	plain := []byte(content)
	checksum := sha256.Sum256(plain)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(plain); err != nil {
		return nil, 0, "", fmt.Errorf("compress session dump: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, 0, "", fmt.Errorf("finish session dump compression: %w", err)
	}
	return compressed.Bytes(), int64(len(plain)), fmt.Sprintf("%x", checksum), nil
}

func decodeSessionDump(encoding string, blob []byte) (string, error) {
	if encoding != sessionDumpEncoding {
		return "", fmt.Errorf("unsupported session dump encoding %q", encoding)
	}
	reader, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return "", fmt.Errorf("open compressed payload: %w", err)
	}
	defer reader.Close()
	plain, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read compressed payload: %w", err)
	}
	return string(plain), nil
}

func shortSessionDumpID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
