ALTER TABLE user_profiles
    DROP CONSTRAINT IF EXISTS user_profiles_bottom_navigation_four_items;

ALTER TABLE user_profiles
    ADD CONSTRAINT user_profiles_bottom_navigation_items
    CHECK (
        jsonb_typeof(bottom_navigation) = 'array'
        AND jsonb_array_length(bottom_navigation) BETWEEN 2 AND 6
        AND bottom_navigation @> '["card", "profile"]'::jsonb
    );
