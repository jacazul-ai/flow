-- +goose Up
ALTER TABLE tasks ADD COLUMN bucket TEXT NOT NULL DEFAULT 'active';

-- +goose Down
-- SQLite cannot safely drop a column without rebuilding the tasks table.
