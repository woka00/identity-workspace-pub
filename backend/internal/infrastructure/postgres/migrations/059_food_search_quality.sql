CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS food_catalog_search_text_simple_idx
    ON food_catalog USING gin (to_tsvector('simple', replace(lower(name || ' ' || brand_name), 'ё', 'е')));

CREATE INDEX IF NOT EXISTS food_catalog_search_text_russian_idx
    ON food_catalog USING gin (to_tsvector('russian', replace(lower(name || ' ' || brand_name), 'ё', 'е')));

CREATE INDEX IF NOT EXISTS food_catalog_search_text_trgm_idx
    ON food_catalog USING gin ((replace(lower(name || ' ' || brand_name), 'ё', 'е')) gin_trgm_ops);

DROP INDEX IF EXISTS food_catalog_name_search_idx;
DROP INDEX IF EXISTS food_catalog_name_search_ru_idx;

CREATE INDEX IF NOT EXISTS user_nutrition_entries_food_usage_idx
    ON user_nutrition_entries(user_id, food_id, updated_at DESC)
    WHERE food_id IS NOT NULL;
