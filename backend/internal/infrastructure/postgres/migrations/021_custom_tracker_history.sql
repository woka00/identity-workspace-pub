-- Дневная история пользовательских трекеров для графиков статистики.
-- Текущее значение в user_custom_trackers остаётся источником общего прогресса.
CREATE TABLE IF NOT EXISTS user_custom_tracker_entries (
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tracker_id   BIGINT NOT NULL REFERENCES user_custom_trackers(id) ON DELETE CASCADE,
    tracked_on   DATE NOT NULL,
    value        NUMERIC(14, 3) NOT NULL CHECK (value >= 0 AND value <= 1000000000),
    target_value NUMERIC(14, 3) NOT NULL CHECK (target_value > 0 AND target_value <= 1000000000),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, tracker_id, tracked_on)
);

CREATE INDEX IF NOT EXISTS user_custom_tracker_entries_user_date_idx
    ON user_custom_tracker_entries (user_id, tracked_on DESC, tracker_id);
