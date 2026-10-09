-- Пользовательский порядок активных задач внутри каждого дня.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS sort_order BIGINT NOT NULL DEFAULT 0;

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY user_id, due_date
               ORDER BY due_time NULLS LAST, created_at, id
           ) AS position
    FROM tasks
    WHERE status = 'todo'
)
UPDATE tasks AS task
SET sort_order = ranked.position
FROM ranked
WHERE task.id = ranked.id
  AND task.sort_order = 0;

CREATE INDEX IF NOT EXISTS tasks_user_date_order_idx
    ON tasks (user_id, due_date, sort_order, id)
    WHERE status = 'todo';
