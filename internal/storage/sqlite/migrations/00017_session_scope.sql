-- +goose Up
-- scope tracks whether a session is in the project's global focus or in its
-- own independent focus, and keys each handoff by session and scope. Every
-- earlier sessions row meant an independent focus, so existing rows and dumps
-- become 'ind'; the project's own 'global' session is 'global'.
ALTER TABLE sessions ADD COLUMN scope TEXT NOT NULL DEFAULT 'ind';
UPDATE sessions SET scope = 'global' WHERE session_id = 'global';

ALTER TABLE session_dumps ADD COLUMN scope TEXT NOT NULL DEFAULT 'ind';
UPDATE session_dumps SET scope = 'global' WHERE session_id = 'global';

DROP INDEX IF EXISTS session_dumps_lookup_idx;
CREATE INDEX IF NOT EXISTS session_dumps_lookup_idx
    ON session_dumps (project_id, session_id, scope, sequence);

-- +goose Down
DROP INDEX IF EXISTS session_dumps_lookup_idx;
CREATE INDEX IF NOT EXISTS session_dumps_lookup_idx
    ON session_dumps (project_id, session_id, sequence);
ALTER TABLE session_dumps DROP COLUMN scope;
ALTER TABLE sessions DROP COLUMN scope;
