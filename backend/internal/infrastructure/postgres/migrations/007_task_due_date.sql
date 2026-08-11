-- Необязательная календарная дата выполнения задачи.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS due_date DATE;

CREATE INDEX IF NOT EXISTS tasks_due_date_idx
    ON tasks (due_date) WHERE due_date IS NOT NULL;
