-- Выбранные дни недели для еженедельных повторений (бит 0 = понедельник, бит 6 = воскресенье).
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurrence_weekdays SMALLINT NOT NULL DEFAULT 0;

ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_recurrence_weekdays_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_recurrence_weekdays_check
    CHECK (recurrence_weekdays BETWEEN 0 AND 127);
