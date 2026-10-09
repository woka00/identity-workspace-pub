-- Each row remains a worked interval, so pauses never contribute to statistics.
ALTER TABLE user_time_sessions
    ADD COLUMN paused BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN carried_seconds NUMERIC NOT NULL DEFAULT 0 CHECK (carried_seconds >= 0),
    ADD CONSTRAINT user_time_sessions_paused_ended CHECK (NOT paused OR ended_at IS NOT NULL);

CREATE UNIQUE INDEX user_time_sessions_one_selected_uidx
    ON user_time_sessions (user_id)
    WHERE ended_at IS NULL OR paused;
