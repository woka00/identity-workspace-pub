-- Historical single-user profile schema retained for upgrade compatibility.
-- Миграция приведена к финальной форме; последующие миграции остаются
-- совместимыми с базами, созданными ранней RPG-версией приложения.
CREATE TABLE IF NOT EXISTS state (
    id           INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    name         TEXT NOT NULL DEFAULT 'DEMO',
    surname      TEXT NOT NULL DEFAULT '',
    occupation   TEXT NOT NULL DEFAULT 'SOFTWARE DEVELOPER',
    sex          TEXT NOT NULL DEFAULT '',
    dob          TEXT NOT NULL DEFAULT '',
    expiry       TEXT NOT NULL DEFAULT '',
    photo        TEXT NOT NULL DEFAULT ''
);

INSERT INTO state (id)
SELECT 1
WHERE NOT EXISTS (SELECT 1 FROM state WHERE id = 1);
