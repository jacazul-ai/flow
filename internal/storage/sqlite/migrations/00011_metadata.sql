-- +goose Up
ALTER TABLE initiatives ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE tasks ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
-- SQLite keeps metadata columns during rollback because dropping columns
-- requires rebuilding the tables.
