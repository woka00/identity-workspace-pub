-- Production security metadata. Existing migrations remain immutable and compatible.
-- Bootstrap development passwords are considered unsafe for public deployment until the
-- owner rotates or disables every enabled account through the admin CLI.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS password_rotated_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS users_enabled_rotation_idx
    ON users (is_enabled, password_rotated_at);

-- Keep session cleanup and idle-session queries efficient.
CREATE INDEX IF NOT EXISTS auth_sessions_last_seen_idx
    ON auth_sessions (last_seen_at);
