-- +goose Up
ALTER TABLE tasks ADD COLUMN position INTEGER NOT NULL DEFAULT 0;

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY initiative_id
               ORDER BY created_at, id
           ) AS position
    FROM tasks
)
UPDATE tasks
SET position = (
    SELECT ranked.position
    FROM ranked
    WHERE ranked.id = tasks.id
);

CREATE UNIQUE INDEX tasks_initiative_position_idx
    ON tasks (initiative_id, position);

-- +goose Down
DROP INDEX IF EXISTS tasks_initiative_position_idx;
