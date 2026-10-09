-- Repeating Web Push reminders while a time-tracker activity is running.
ALTER TABLE user_time_activities
    ADD COLUMN IF NOT EXISTS reminder_interval_minutes INTEGER,
    ADD COLUMN IF NOT EXISTS reminder_message TEXT;

ALTER TABLE user_time_activities
    DROP CONSTRAINT IF EXISTS user_time_activities_reminder_interval_check;
ALTER TABLE user_time_activities
    ADD CONSTRAINT user_time_activities_reminder_interval_check
    CHECK (reminder_interval_minutes IS NULL OR reminder_interval_minutes BETWEEN 5 AND 240);

ALTER TABLE user_time_activities
    DROP CONSTRAINT IF EXISTS user_time_activities_reminder_message_check;
ALTER TABLE user_time_activities
    ADD CONSTRAINT user_time_activities_reminder_message_check
    CHECK (reminder_message IS NULL OR char_length(btrim(reminder_message)) BETWEEN 1 AND 160);

ALTER TABLE user_time_sessions
    ADD COLUMN IF NOT EXISTS next_reminder_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reminder_claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reminder_attempted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS user_time_sessions_due_reminder_idx
    ON user_time_sessions (next_reminder_at)
    WHERE ended_at IS NULL AND next_reminder_at IS NOT NULL;
