-- Фото (data URL, jpeg) и дата рождения на карте.
ALTER TABLE state ADD COLUMN IF NOT EXISTS photo TEXT NOT NULL DEFAULT '';
ALTER TABLE state ADD COLUMN IF NOT EXISTS dob   TEXT NOT NULL DEFAULT '';
