CREATE TABLE IF NOT EXISTS user_time_activities (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 60),
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS user_time_activities_active_name_uidx
    ON user_time_activities (user_id, lower(btrim(name)))
    WHERE archived_at IS NULL;

CREATE INDEX IF NOT EXISTS user_time_activities_order_idx
    ON user_time_activities (user_id, sort_order, created_at, id)
    WHERE archived_at IS NULL;

CREATE TABLE IF NOT EXISTS user_time_sessions (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id BIGINT NOT NULL REFERENCES user_time_activities(id) ON DELETE CASCADE,
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ended_at IS NULL OR ended_at >= started_at)
);

CREATE UNIQUE INDEX IF NOT EXISTS user_time_sessions_one_active_uidx
    ON user_time_sessions (user_id)
    WHERE ended_at IS NULL;

CREATE INDEX IF NOT EXISTS user_time_sessions_user_time_idx
    ON user_time_sessions (user_id, started_at DESC);

CREATE INDEX IF NOT EXISTS user_time_sessions_activity_time_idx
    ON user_time_sessions (activity_id, started_at DESC);
