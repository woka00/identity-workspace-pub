-- Daily Web Push reminders for built-in and custom tracker cards.
CREATE TABLE IF NOT EXISTS user_tracker_reminders (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tracker_key TEXT NOT NULL,
    remind_time TIME NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_sent_on DATE,
    reminder_claimed_at TIMESTAMPTZ,
    reminder_attempted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, tracker_key),
    CONSTRAINT user_tracker_reminders_key_check CHECK (
        tracker_key IN ('calories', 'water', 'weight')
        OR tracker_key ~ '^custom:[1-9][0-9]*$'
    )
);

CREATE INDEX IF NOT EXISTS user_tracker_reminders_due_idx
    ON user_tracker_reminders (remind_time)
    WHERE enabled;
