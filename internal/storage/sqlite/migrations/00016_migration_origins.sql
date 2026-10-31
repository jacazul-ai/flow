-- +goose Up
-- migration_origins records which legacy source a project database was
-- migrated from, written in the same transaction as the imported records.
CREATE TABLE IF NOT EXISTS migration_origins (
    project_id TEXT PRIMARY KEY,
    origin_path TEXT NOT NULL,
    migrated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS migration_origins;
