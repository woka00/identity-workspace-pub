-- История показателей для будущих графиков.
-- Вес и вода хранятся по календарным датам: повторное сохранение дня
-- исправляет существующую точку, не создавая дубликат.
CREATE TABLE IF NOT EXISTS tracker_settings (
    id         INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    water_goal SMALLINT NOT NULL DEFAULT 8 CHECK (water_goal BETWEEN 1 AND 30)
);

INSERT INTO tracker_settings (id)
SELECT 1
WHERE NOT EXISTS (SELECT 1 FROM tracker_settings WHERE id = 1);

CREATE TABLE IF NOT EXISTS tracker_weight_entries (
    tracked_on DATE PRIMARY KEY,
    weight_kg  NUMERIC(5, 1) NOT NULL CHECK (weight_kg BETWEEN 20 AND 500),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tracker_water_entries (
    tracked_on    DATE PRIMARY KEY,
    glasses       SMALLINT NOT NULL CHECK (glasses BETWEEN 0 AND 99),
    goal_glasses  SMALLINT NOT NULL CHECK (goal_glasses BETWEEN 1 AND 30),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
