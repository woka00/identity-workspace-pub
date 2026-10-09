ALTER TABLE user_profiles
    ADD COLUMN work_display_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN work_avatar TEXT NOT NULL DEFAULT '',
    ADD COLUMN work_show_name BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN work_show_avatar BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE user_profiles
    ADD CONSTRAINT user_profiles_work_display_name_length CHECK (char_length(work_display_name) <= 80);
