-- Портфолио проектов и свободные задачи.
ALTER TABLE goals ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE goals ADD COLUMN IF NOT EXISTS summary     TEXT NOT NULL DEFAULT '';
ALTER TABLE goals ADD COLUMN IF NOT EXISTS pinned      BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS tasks (
    id           BIGSERIAL PRIMARY KEY,
    title        TEXT NOT NULL,
    category     TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'todo'
                 CHECK (status IN ('todo', 'doing', 'done')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    is_milestone BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS tasks_status_created_idx
    ON tasks (status, created_at DESC);
CREATE INDEX IF NOT EXISTS tasks_category_idx
    ON tasks (category) WHERE category <> '';
CREATE INDEX IF NOT EXISTS goals_portfolio_idx
    ON goals (pinned DESC, completed_at DESC)
    WHERE completed_at IS NOT NULL;
