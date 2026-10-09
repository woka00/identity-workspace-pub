CREATE TABLE IF NOT EXISTS food_catalog (
    id BIGSERIAL PRIMARY KEY,
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL,
    barcode TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    brand_name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    calories_per_100g DOUBLE PRECISION NOT NULL DEFAULT 0,
    carbohydrate_per_100g DOUBLE PRECISION NOT NULL DEFAULT 0,
    protein_per_100g DOUBLE PRECISION NOT NULL DEFAULT 0,
    fat_per_100g DOUBLE PRECISION NOT NULL DEFAULT 0,
    servings JSONB NOT NULL DEFAULT '[]'::jsonb,
    data_quality DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    cached_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, external_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS food_catalog_provider_barcode_idx ON food_catalog(provider, barcode) WHERE barcode <> '';

CREATE INDEX IF NOT EXISTS food_catalog_name_search_idx ON food_catalog USING gin (to_tsvector('simple', lower(name || ' ' || brand_name)));

CREATE TABLE IF NOT EXISTS user_nutrition_entries (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entry_date DATE NOT NULL,
    meal TEXT NOT NULL CHECK (meal IN ('breakfast','lunch','dinner','other')),
    food_id BIGINT REFERENCES food_catalog(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    number_of_units DOUBLE PRECISION NOT NULL CHECK (number_of_units > 0),
    calories DOUBLE PRECISION NOT NULL DEFAULT 0,
    carbohydrate DOUBLE PRECISION NOT NULL DEFAULT 0,
    protein DOUBLE PRECISION NOT NULL DEFAULT 0,
    fat DOUBLE PRECISION NOT NULL DEFAULT 0,
    serving_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS user_nutrition_entries_date_idx ON user_nutrition_entries(user_id, entry_date);
