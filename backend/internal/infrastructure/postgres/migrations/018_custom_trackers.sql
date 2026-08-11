-- Пользовательские трекеры-плашки.
CREATE TABLE IF NOT EXISTS user_custom_trackers (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    target_value  NUMERIC(14, 3) NOT NULL CHECK (target_value > 0 AND target_value <= 1000000000),
    step_value    NUMERIC(14, 3) NOT NULL CHECK (step_value > 0 AND step_value <= 1000000000),
    current_value NUMERIC(14, 3) NOT NULL DEFAULT 0 CHECK (current_value >= 0 AND current_value <= 1000000000),
    icon          TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS user_custom_trackers_user_idx ON user_custom_trackers (user_id, created_at);
