-- Barcode-to-candidate links used to live only in the private alias table.
-- Materialise them as private catalogue rows so they are reviewable by an
-- administrator and remain directly searchable by barcode.
INSERT INTO food_catalog (
    provider, external_id, barcode, name, brand_name, description,
    calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g,
    servings, data_quality, cached_at, owner_user_id
)
SELECT 'local',
       'barcode-link-' || link.user_id::text || '-' || source.id::text || '-' || md5(link.barcode),
       link.barcode, source.name, source.brand_name, source.description,
       source.calories_per_100g, source.carbohydrate_per_100g,
       source.protein_per_100g, source.fat_per_100g,
       source.servings, 1, now(), link.user_id
FROM user_food_barcode_links AS link
JOIN food_catalog AS source ON source.id=link.food_id
WHERE source.provider <> 'local'
   OR source.owner_user_id IS DISTINCT FROM link.user_id
   OR source.barcode <> link.barcode
ON CONFLICT (owner_user_id, barcode)
WHERE provider='local' AND owner_user_id IS NOT NULL AND barcode <> '' DO UPDATE SET
    name=EXCLUDED.name,
    brand_name=EXCLUDED.brand_name,
    description=EXCLUDED.description,
    calories_per_100g=EXCLUDED.calories_per_100g,
    carbohydrate_per_100g=EXCLUDED.carbohydrate_per_100g,
    protein_per_100g=EXCLUDED.protein_per_100g,
    fat_per_100g=EXCLUDED.fat_per_100g,
    servings=EXCLUDED.servings,
    data_quality=1,
    cached_at=now();

UPDATE user_food_barcode_links AS link
SET food_id=private_food.id,
    created_at=now()
FROM food_catalog AS private_food
WHERE private_food.provider='local'
  AND private_food.owner_user_id=link.user_id
  AND private_food.barcode=link.barcode
  AND link.food_id<>private_food.id;
