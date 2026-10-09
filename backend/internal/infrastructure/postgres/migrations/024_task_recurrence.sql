-- Повторяющиеся задачи. Каждый новый экземпляр ссылается на завершённый предыдущий.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurrence_type TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurrence_interval SMALLINT NOT NULL DEFAULT 1;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurrence_end_date DATE;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS generated_from_task_id BIGINT REFERENCES tasks(id) ON DELETE SET NULL;

ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_recurrence_type_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_recurrence_type_check
    CHECK (recurrence_type IN ('', 'daily', 'weekly', 'monthly'));
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_recurrence_interval_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_recurrence_interval_check
    CHECK (recurrence_interval BETWEEN 1 AND 365);

CREATE UNIQUE INDEX IF NOT EXISTS tasks_generated_from_unique
    ON tasks (user_id, generated_from_task_id)
    WHERE generated_from_task_id IS NOT NULL;
