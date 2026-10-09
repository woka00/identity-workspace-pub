UPDATE user_profiles
SET bottom_navigation = '["card", "tasks", "tracker", "profile"]'::jsonb
WHERE NOT (
    bottom_navigation ? 'card'
    AND bottom_navigation ? 'tasks'
    AND bottom_navigation ? 'profile'
);

ALTER TABLE user_profiles
    ADD CONSTRAINT user_profiles_bottom_navigation_required_items
    CHECK (bottom_navigation ?& ARRAY['card', 'tasks', 'profile']);
