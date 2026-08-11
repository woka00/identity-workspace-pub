-- Пользовательские аккаунты, серверные сессии и изоляция данных.
-- Старые однопользовательские таблицы сохраняются как источник данных для
-- первого зарегистрированного пользователя.
CREATE TABLE IF NOT EXISTS users (
    id               BIGSERIAL PRIMARY KEY,
    login            TEXT NOT NULL,
    login_normalized TEXT NOT NULL UNIQUE,
    password_hash    TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   CHAR(64) NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS auth_sessions_user_idx ON auth_sessions (user_id);
CREATE INDEX IF NOT EXISTS auth_sessions_expiry_idx ON auth_sessions (expires_at);

CREATE TABLE IF NOT EXISTS user_profiles (
    user_id      BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL DEFAULT 'USER',
    surname      TEXT NOT NULL DEFAULT '',
    occupation   TEXT NOT NULL DEFAULT '',
    sex          TEXT NOT NULL DEFAULT '',
    dob          TEXT NOT NULL DEFAULT '',
    expiry       TEXT NOT NULL DEFAULT '',
    photo        TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS user_tracker_settings (
    user_id      BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    water_goal   SMALLINT NOT NULL DEFAULT 8 CHECK (water_goal BETWEEN 1 AND 30),
    calorie_goal INTEGER NOT NULL DEFAULT 2000 CHECK (calorie_goal BETWEEN 500 AND 10000)
);

CREATE TABLE IF NOT EXISTS user_tracker_weight_entries (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tracked_on DATE NOT NULL,
    weight_kg  NUMERIC(5, 1) NOT NULL CHECK (weight_kg BETWEEN 20 AND 500),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, tracked_on)
);

CREATE TABLE IF NOT EXISTS user_tracker_water_entries (
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tracked_on   DATE NOT NULL,
    glasses      SMALLINT NOT NULL CHECK (glasses BETWEEN 0 AND 99),
    goal_glasses SMALLINT NOT NULL CHECK (goal_glasses BETWEEN 1 AND 30),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, tracked_on)
);

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE goals ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS tasks_user_status_idx ON tasks (user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS goals_user_portfolio_idx ON goals (user_id, pinned DESC, completed_at DESC);

CREATE TABLE IF NOT EXISTS user_fatsecret_connections (
    user_id            BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    oauth_token        TEXT NOT NULL,
    oauth_token_secret TEXT NOT NULL,
    connected_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_fatsecret_oauth_requests (
    oauth_token        TEXT PRIMARY KEY,
    user_id            BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    oauth_token_secret TEXT NOT NULL,
    return_to          TEXT NOT NULL DEFAULT '/',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS user_fatsecret_oauth_requests_created_idx
    ON user_fatsecret_oauth_requests (created_at);
