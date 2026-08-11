-- TickTick OAuth 2.0 connection and per-task synchronization.
-- Existing task rows stay compatible: description is optional and empty by default.
ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS user_ticktick_connections (
    user_id       BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    access_token  TEXT NOT NULL,
    project_id    TEXT NOT NULL,
    project_name  TEXT NOT NULL DEFAULT 'identity workspace',
    connected_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_ticktick_oauth_states (
    state         TEXT PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    callback_url  TEXT NOT NULL,
    return_to     TEXT NOT NULL DEFAULT '/',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS user_ticktick_oauth_states_created_idx
    ON user_ticktick_oauth_states (created_at);

CREATE TABLE IF NOT EXISTS user_ticktick_task_links (
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    task_id          BIGINT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    ticktick_task_id TEXT NOT NULL DEFAULT '',
    project_id       TEXT NOT NULL DEFAULT '',
    sync_status      TEXT NOT NULL DEFAULT 'pending'
                     CHECK (sync_status IN ('pending', 'synced', 'error')),
    last_error       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS user_ticktick_task_links_remote_idx
    ON user_ticktick_task_links (user_id, ticktick_task_id)
    WHERE ticktick_task_id <> '';

CREATE INDEX IF NOT EXISTS user_ticktick_task_links_user_status_idx
    ON user_ticktick_task_links (user_id, sync_status, updated_at);
