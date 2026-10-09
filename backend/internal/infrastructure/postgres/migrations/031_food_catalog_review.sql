-- Fast macro matching, private barcode aliases and explicit moderation of
-- user-created foods before they become visible to every account.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE users
SET is_admin=TRUE
WHERE id=(SELECT MIN(id) FROM users)
  AND NOT EXISTS (SELECT 1 FROM users WHERE is_admin=TRUE);

CREATE TABLE IF NOT EXISTS user_food_barcode_links (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    barcode    TEXT NOT NULL,
    food_id    BIGINT NOT NULL REFERENCES food_catalog(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, barcode)
);

CREATE INDEX IF NOT EXISTS user_food_barcode_links_food_idx
    ON user_food_barcode_links(food_id);

CREATE INDEX IF NOT EXISTS food_catalog_macro_candidate_idx
    ON food_catalog (
        calories_per_100g,
        protein_per_100g,
        fat_per_100g,
        carbohydrate_per_100g
    )
    INCLUDE (provider, owner_user_id, name, brand_name);

CREATE TABLE IF NOT EXISTS food_catalog_promotions (
    source_food_id      BIGINT PRIMARY KEY REFERENCES food_catalog(id) ON DELETE CASCADE,
    global_food_id      BIGINT NOT NULL REFERENCES food_catalog(id) ON DELETE CASCADE,
    reviewed_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    promoted_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS food_catalog_promotions_global_idx
    ON food_catalog_promotions(global_food_id);
