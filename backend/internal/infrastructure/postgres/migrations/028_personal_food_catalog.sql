-- Provider products remain shared. Locally created products and corrections
-- belong to one user and must never affect another user's barcode lookup.
ALTER TABLE food_catalog
    ADD COLUMN IF NOT EXISTS owner_user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;

DROP INDEX IF EXISTS food_catalog_provider_barcode_idx;

-- Preserve legacy local products that were already used in nutrition diaries.
-- When several users used the same formerly-global row, create a private copy
-- for every user except the first one and repoint their immutable diary links.
CREATE TEMP TABLE legacy_local_food_users ON COMMIT DROP AS
SELECT DISTINCT entry.food_id, entry.user_id
FROM user_nutrition_entries AS entry
JOIN food_catalog AS food ON food.id=entry.food_id
WHERE food.provider='local' AND food.owner_user_id IS NULL;

INSERT INTO food_catalog (
    provider, external_id, barcode, name, brand_name, description,
    calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g,
    servings, data_quality, cached_at, owner_user_id
)
SELECT food.provider,
       food.external_id || '-owner-' || owner.user_id::text,
       food.barcode, food.name, food.brand_name, food.description,
       food.calories_per_100g, food.carbohydrate_per_100g,
       food.protein_per_100g, food.fat_per_100g,
       food.servings, food.data_quality, food.cached_at, owner.user_id
FROM legacy_local_food_users AS owner
JOIN food_catalog AS food ON food.id=owner.food_id
WHERE owner.user_id <> (
    SELECT MIN(first_owner.user_id)
    FROM legacy_local_food_users AS first_owner
    WHERE first_owner.food_id=owner.food_id
);

UPDATE user_nutrition_entries AS entry
SET food_id=private_food.id
FROM legacy_local_food_users AS owner
JOIN food_catalog AS original_food ON original_food.id=owner.food_id
JOIN food_catalog AS private_food
  ON private_food.provider='local'
 AND private_food.owner_user_id=owner.user_id
 AND private_food.external_id=original_food.external_id || '-owner-' || owner.user_id::text
WHERE entry.food_id=owner.food_id
  AND entry.user_id=owner.user_id
  AND owner.user_id <> (
      SELECT MIN(first_owner.user_id)
      FROM legacy_local_food_users AS first_owner
      WHERE first_owner.food_id=owner.food_id
  );

UPDATE food_catalog AS food
SET owner_user_id=owner.user_id
FROM (
    SELECT food_id, MIN(user_id) AS user_id
    FROM legacy_local_food_users
    GROUP BY food_id
) AS owner
WHERE food.id=owner.food_id;

CREATE UNIQUE INDEX IF NOT EXISTS food_catalog_owner_barcode_idx
    ON food_catalog(owner_user_id, barcode)
    WHERE provider='local' AND owner_user_id IS NOT NULL AND barcode <> '';

CREATE UNIQUE INDEX IF NOT EXISTS food_catalog_shared_provider_barcode_idx
    ON food_catalog(provider, barcode)
    WHERE provider <> 'local' AND barcode <> '';

CREATE INDEX IF NOT EXISTS food_catalog_owner_idx
    ON food_catalog(owner_user_id)
    WHERE owner_user_id IS NOT NULL;
