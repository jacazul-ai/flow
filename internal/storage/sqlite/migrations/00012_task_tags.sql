-- +goose Up
CREATE TABLE IF NOT EXISTS task_tags (
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    PRIMARY KEY (task_id, tag)
);

CREATE INDEX IF NOT EXISTS task_tags_tag_idx
    ON task_tags (tag, task_id);

-- +goose Down
DROP INDEX IF EXISTS task_tags_tag_idx;
DROP TABLE IF EXISTS task_tags;
