-- Дневная норма калорий для карточки питания.
ALTER TABLE tracker_settings
    ADD COLUMN IF NOT EXISTS calorie_goal INTEGER NOT NULL DEFAULT 2000;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'tracker_settings_calorie_goal_check'
    ) THEN
        ALTER TABLE tracker_settings
            ADD CONSTRAINT tracker_settings_calorie_goal_check
            CHECK (calorie_goal BETWEEN 500 AND 10000);
    END IF;
END $$;
