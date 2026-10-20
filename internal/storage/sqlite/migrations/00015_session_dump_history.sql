-- +goose Up
ALTER TABLE sessions ADD COLUMN state TEXT NOT NULL DEFAULT 'active';
ALTER TABLE sessions ADD COLUMN purged_at TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS session_dumps (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    delivered_at TEXT NOT NULL DEFAULT '',
    acknowledged_at TEXT NOT NULL DEFAULT '',
    focus_summary TEXT NOT NULL DEFAULT '',
    content_encoding TEXT NOT NULL,
    content_blob BLOB NOT NULL,
    content_size INTEGER NOT NULL,
    content_sha256 TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (project_id, session_id, sequence)
);

CREATE INDEX IF NOT EXISTS session_dumps_lookup_idx
    ON session_dumps (project_id, session_id, sequence);

-- +goose Down
DROP INDEX IF EXISTS session_dumps_lookup_idx;
DROP TABLE IF EXISTS session_dumps;
