-- Настройка GitHub-трекера и серверный кэш публичной активности.
-- Username и кэш принадлежат конкретному пользователю Identity Workspace.
CREATE TABLE IF NOT EXISTS user_github_tracker_settings (
    user_id             BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    github_username     TEXT NOT NULL CHECK (char_length(github_username) BETWEEN 1 AND 39),
    cached_activity     JSONB NOT NULL DEFAULT '{}'::jsonb,
    activity_fetched_at TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
