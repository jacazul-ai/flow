-- +goose Up
CREATE TABLE IF NOT EXISTS workflow_history (
    id TEXT PRIMARY KEY,
    task_id TEXT REFERENCES tasks(id) ON DELETE CASCADE,
    initiative_id TEXT REFERENCES initiatives(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    source_event_id TEXT NOT NULL,
    sequence INTEGER NOT NULL DEFAULT 0,
    event_type TEXT NOT NULL,
    property TEXT NOT NULL DEFAULT '',
    old_value TEXT NOT NULL DEFAULT '',
    new_value TEXT NOT NULL DEFAULT '',
    occurred_at TEXT NOT NULL,
    actor TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    recorded_at TEXT NOT NULL,
    CHECK (task_id IS NOT NULL OR initiative_id IS NOT NULL),
    UNIQUE (source, source_event_id)
);

CREATE INDEX IF NOT EXISTS workflow_history_task_order_idx
    ON workflow_history (task_id, occurred_at, sequence, id);

CREATE INDEX IF NOT EXISTS workflow_history_initiative_order_idx
    ON workflow_history (initiative_id, occurred_at, sequence, id);

-- +goose Down
DROP INDEX IF EXISTS workflow_history_initiative_order_idx;
DROP INDEX IF EXISTS workflow_history_task_order_idx;
DROP TABLE IF EXISTS workflow_history;
