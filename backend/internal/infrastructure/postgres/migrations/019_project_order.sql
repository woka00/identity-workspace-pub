-- Пользовательский порядок проектов.
ALTER TABLE goals ADD COLUMN IF NOT EXISTS sort_order BIGINT NOT NULL DEFAULT 0;

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY user_id
               ORDER BY completed_at NULLS FIRST, deadline NULLS LAST, created_at DESC, id DESC
           ) AS position
    FROM goals
)
UPDATE goals AS g
SET sort_order = ranked.position
FROM ranked
WHERE g.id = ranked.id
  AND g.sort_order = 0;

CREATE INDEX IF NOT EXISTS goals_user_sort_order_idx
    ON goals (user_id, sort_order, id);
