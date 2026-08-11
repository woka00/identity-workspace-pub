-- Проекты и крупные достижения. Связи с рабочими задачами хранятся в JSONB.
CREATE TABLE IF NOT EXISTS goals (
    id                 BIGSERIAL PRIMARY KEY,
    title              TEXT NOT NULL,
    current_value      DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (current_value >= 0),
    target_value       DOUBLE PRECISION NOT NULL CHECK (target_value > 0),
    unit               TEXT NOT NULL DEFAULT '',
    deadline           DATE,
    related_task_ids   JSONB NOT NULL DEFAULT '[]'::jsonb,
    completed_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS goals_deadline_idx ON goals (deadline)
WHERE completed_at IS NULL;
