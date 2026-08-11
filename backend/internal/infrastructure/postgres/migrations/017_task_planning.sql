-- Планирование задач, категории и Web Push-напоминания.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS due_time TIME;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS reminder_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS reminder_sent_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS reminder_claimed_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS reminder_attempted_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS priority SMALLINT NOT NULL DEFAULT 0;

UPDATE tasks SET priority=3 WHERE is_milestone=TRUE AND priority=0;

ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_priority_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_priority_check CHECK (priority BETWEEN 0 AND 3);
CREATE INDEX IF NOT EXISTS tasks_due_reminder_idx
    ON tasks (reminder_at)
    WHERE status='todo' AND reminder_at IS NOT NULL AND reminder_sent_at IS NULL;

CREATE TABLE IF NOT EXISTS user_task_categories (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    name_normalized TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, name_normalized)
);

INSERT INTO user_task_categories (user_id, name, name_normalized)
SELECT id, 'Дом', 'дом' FROM users
ON CONFLICT (user_id, name_normalized) DO NOTHING;
INSERT INTO user_task_categories (user_id, name, name_normalized)
SELECT id, 'Работа', 'работа' FROM users
ON CONFLICT (user_id, name_normalized) DO NOTHING;

INSERT INTO user_task_categories (user_id, name, name_normalized)
SELECT DISTINCT user_id, initcap(category), lower(category)
FROM tasks
WHERE user_id IS NOT NULL AND btrim(category)<>''
ON CONFLICT (user_id, name_normalized) DO NOTHING;

CREATE TABLE IF NOT EXISTS user_push_subscriptions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   TEXT NOT NULL,
    p256dh     TEXT NOT NULL,
    auth       TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (endpoint)
);
CREATE INDEX IF NOT EXISTS user_push_subscriptions_user_idx ON user_push_subscriptions (user_id);
