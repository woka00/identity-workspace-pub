-- Закрытый режим разработки: ровно 15 заранее подготовленных аккаунтов.
-- Пароли в базе не хранятся: здесь находятся только PBKDF2-HMAC-SHA256 хеши.
-- Самый ранний существующий пользователь становится avatar01, чтобы сохранить
-- его карточку, задачи, проекты, трекеры и подключение FatSecret.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_enabled BOOLEAN NOT NULL DEFAULT TRUE;

DO $migration$
DECLARE
    owner_id BIGINT;
BEGIN
    LOCK TABLE users IN EXCLUSIVE MODE;

    -- Новые пароли должны сразу завершить все старые сессии.
    DELETE FROM auth_sessions;

    SELECT id INTO owner_id FROM users ORDER BY id LIMIT 1;

    IF owner_id IS NULL THEN
        INSERT INTO users (login, login_normalized, password_hash, is_enabled)
        VALUES ('avatar01', 'avatar01', 'pbkdf2-sha256$180000$Li9L7youqkDHh7wXiMZ23w$IuqZ3h4s5liNdWyvPtSBL/RlZ62zJQGzM7LXDzompwM', TRUE)
        RETURNING id INTO owner_id;
    ELSE
        -- Старые дополнительные аккаунты сохраняются в базе вместе с их данными,
        -- но становятся неактивными и не могут пройти авторизацию.
        UPDATE users
        SET login='disabled_' || id::text,
            login_normalized='disabled_' || id::text,
            is_enabled=FALSE
        WHERE id <> owner_id;

        UPDATE users
        SET login='avatar01',
            login_normalized='avatar01',
            password_hash='pbkdf2-sha256$180000$Li9L7youqkDHh7wXiMZ23w$IuqZ3h4s5liNdWyvPtSBL/RlZ62zJQGzM7LXDzompwM',
            is_enabled=TRUE
        WHERE id=owner_id;
    END IF;

    -- Забираем данные старой однопользовательской версии, если они ещё не были перенесены.
    INSERT INTO user_profiles (user_id, name, surname, occupation, sex, dob, expiry, photo)
    SELECT owner_id, name, surname, occupation, sex, dob, expiry, photo
    FROM state WHERE id=1
    ON CONFLICT (user_id) DO NOTHING;

    INSERT INTO user_tracker_settings (user_id, water_goal, calorie_goal)
    SELECT owner_id, water_goal, calorie_goal
    FROM tracker_settings WHERE id=1
    ON CONFLICT (user_id) DO NOTHING;

    INSERT INTO user_tracker_weight_entries (user_id, tracked_on, weight_kg, updated_at)
    SELECT owner_id, tracked_on, weight_kg, updated_at
    FROM tracker_weight_entries
    ON CONFLICT (user_id, tracked_on) DO NOTHING;

    INSERT INTO user_tracker_water_entries (user_id, tracked_on, glasses, goal_glasses, updated_at)
    SELECT owner_id, tracked_on, glasses, goal_glasses, updated_at
    FROM tracker_water_entries
    ON CONFLICT (user_id, tracked_on) DO NOTHING;

    INSERT INTO user_fatsecret_connections (user_id, oauth_token, oauth_token_secret, connected_at)
    SELECT owner_id, oauth_token, oauth_token_secret, connected_at
    FROM fatsecret_connection WHERE id=1
    ON CONFLICT (user_id) DO NOTHING;

    UPDATE tasks SET user_id=owner_id WHERE user_id IS NULL;
    UPDATE goals SET user_id=owner_id WHERE user_id IS NULL;
END
$migration$;

INSERT INTO users (login, login_normalized, password_hash, is_enabled)
VALUES
    ('avatar01', 'avatar01', 'pbkdf2-sha256$180000$Li9L7youqkDHh7wXiMZ23w$IuqZ3h4s5liNdWyvPtSBL/RlZ62zJQGzM7LXDzompwM', TRUE),
    ('avatar02', 'avatar02', 'pbkdf2-sha256$180000$g/Ej+4geLpN0TRO9+h3PuQ$GiO99z7cpLSNhQdXQtavNoWvMLpXI9ZousrZqbyVjbU', TRUE),
    ('avatar03', 'avatar03', 'pbkdf2-sha256$180000$hNI//cdf2iMgphVFdbChjQ$DT3GsAMGiRrEO5i41yzr1yjAr3yuDAAM+Ab/jP5kkpE', TRUE),
    ('avatar04', 'avatar04', 'pbkdf2-sha256$180000$jLcXs8sscJZYs7LJPGKeog$iezy7G622+PbfxLVm4QsquFbY0BFNFDurtcVYl1siBo', TRUE),
    ('avatar05', 'avatar05', 'pbkdf2-sha256$180000$EpKweiHirFQwU4EgncNcZQ$/rPwlfwvX/r1hmuGDzAusLgKujmKRjinARLukLXYTPM', TRUE),
    ('avatar06', 'avatar06', 'pbkdf2-sha256$180000$skk1lXbdpLaAuw2CPJZPlg$NcG8nVW2C6UM/bKZNx1V7M31Q3YlnhBJQRqXTA1OwH0', TRUE),
    ('avatar07', 'avatar07', 'pbkdf2-sha256$180000$5Jxf4UeUg7snkx6aSyM2vw$+RCPeHxvWoiPc7TWbwcAmqPvhNT4a3b8ZPQNDDc3ipA', TRUE),
    ('avatar08', 'avatar08', 'pbkdf2-sha256$180000$VTcMDmIOe7TiOSs3IdK7Ug$EmsNC7wmQ7552FmFBpwHmKAXXvS8VYkKzpTThJ1ZNJE', TRUE),
    ('avatar09', 'avatar09', 'pbkdf2-sha256$180000$eifXOpOfajIt3Epd8uGtCw$UnLbhfMYmEgeEsduQNwGSOedkLkiauH6VR9DzL57OPA', TRUE),
    ('avatar10', 'avatar10', 'pbkdf2-sha256$180000$Eg0wdzVeOZZKCMZjUMY7nA$N5cek72kfrFa3ZwPE7wXipGfEoOrXEcM81Yz26nhl8M', TRUE),
    ('avatar11', 'avatar11', 'pbkdf2-sha256$180000$LJ7WrN3fogksPLBks2RXyg$oIF9n2k7tUj+zzWFCeJpgHFX8JKxStX9wKhZfZiAFtI', TRUE),
    ('avatar12', 'avatar12', 'pbkdf2-sha256$180000$J7XQWKCSSn854k821sjLng$ALUTknRNhXTN6hznFCPFMagCFYc2rqiyjyRjvOg96Oo', TRUE),
    ('avatar13', 'avatar13', 'pbkdf2-sha256$180000$xpLDiQxeHOquLz+bXIVD7w$JP71/DAxxffxRet4eBMYI+NsKVQn85iyXtyjy7bhp/A', TRUE),
    ('avatar14', 'avatar14', 'pbkdf2-sha256$180000$KubfXHGH5tIog3Q6i/jGsQ$8fktD2gd1JarUYTXnNq2x4ny2d91wegRsncyIblXwlg', TRUE),
    ('avatar15', 'avatar15', 'pbkdf2-sha256$180000$qaGnHODmX6hI9m58PQB6PQ$Dd6D5cIzFd7W+17iIUsItKAhh2FNXaOlkyMvjTLgUII', TRUE)
ON CONFLICT (login_normalized) DO UPDATE
SET login=EXCLUDED.login,
    password_hash=EXCLUDED.password_hash,
    is_enabled=TRUE;

-- Только фиксированный список может пройти авторизацию. Неизвестные прежние
-- аккаунты остаются в базе выключенными, чтобы не терять их данные.
UPDATE users
SET is_enabled=FALSE
WHERE login_normalized NOT IN ('avatar01', 'avatar02', 'avatar03', 'avatar04', 'avatar05', 'avatar06', 'avatar07', 'avatar08', 'avatar09', 'avatar10', 'avatar11', 'avatar12', 'avatar13', 'avatar14', 'avatar15');

INSERT INTO user_profiles (user_id, name)
SELECT id, upper(login) FROM users WHERE is_enabled
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO user_tracker_settings (user_id)
SELECT id FROM users WHERE is_enabled
ON CONFLICT (user_id) DO NOTHING;
