ALTER TABLE user_profiles
    ADD COLUMN bottom_navigation JSONB NOT NULL
    DEFAULT '["card", "tasks", "tracker", "profile"]'::jsonb;

ALTER TABLE user_profiles
    ADD CONSTRAINT user_profiles_bottom_navigation_four_items
    CHECK (
        jsonb_typeof(bottom_navigation) = 'array'
        AND jsonb_array_length(bottom_navigation) = 4
    );
