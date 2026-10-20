-- +goose Up
CREATE TABLE IF NOT EXISTS migration_issues (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS migration_issues_project_idx
    ON migration_issues (project_id, severity, id);

-- +goose Down
DROP INDEX IF EXISTS migration_issues_project_idx;
DROP TABLE IF EXISTS migration_issues;
