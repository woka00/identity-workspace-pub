-- Public development bootstrap.
--
-- This migration deliberately contains one documented development-only user:
--   login: demo
--   password: identity-workspace-demo
--
-- Production startup remains blocked by migration 016 until this password is
-- replaced with the admin CLI or the account is disabled. Do not reuse these
-- credentials outside a local disposable environment.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_enabled BOOLEAN NOT NULL DEFAULT TRUE;

DO $migration$
DECLARE
    owner_id BIGINT;
    demo_password_hash TEXT := 'pbkdf2-sha256$600000$cT6yLDn3kyBHCRGA/ZTiVg$9OvW8wm3Gz+lLoklkp+zOqruZaYAL+r9RucCzgh3KfI';
BEGIN
    LOCK TABLE users IN EXCLUSIVE MODE;

    -- A credential bootstrap invalidates every session from an older local schema.
    DELETE FROM auth_sessions;

    SELECT id INTO owner_id FROM users ORDER BY id LIMIT 1;

    IF owner_id IS NULL THEN
        INSERT INTO users (login, login_normalized, password_hash, is_enabled)
        VALUES ('demo', 'demo', demo_password_hash, TRUE)
        RETURNING id INTO owner_id;
    ELSE
        -- Preserve older local data but disable every account except the first.
        UPDATE users
        SET login='disabled-local-' || id::text,
            login_normalized='disabled-local-' || id::text,
            is_enabled=FALSE
        WHERE id <> owner_id;

        UPDATE users
        SET login='demo',
            login_normalized='demo',
            password_hash=demo_password_hash,
            is_enabled=TRUE
        WHERE id=owner_id;
    END IF;

    -- Import data from the original single-user schema when upgrading a local DB.
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

INSERT INTO user_profiles (user_id, name, occupation)
SELECT id, 'DEMO', 'SOFTWARE DEVELOPER' FROM users WHERE login_normalized='demo'
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO user_tracker_settings (user_id)
SELECT id FROM users WHERE login_normalized='demo'
ON CONFLICT (user_id) DO NOTHING;
