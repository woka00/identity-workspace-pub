CREATE TABLE food_catalog_review_edits (
    source_food_id BIGINT PRIMARY KEY REFERENCES food_catalog(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    brand_name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    barcode TEXT NOT NULL DEFAULT '',
    calories_per_100g DOUBLE PRECISION NOT NULL CHECK (calories_per_100g BETWEEN 0 AND 1000),
    carbohydrate_per_100g DOUBLE PRECISION NOT NULL CHECK (carbohydrate_per_100g BETWEEN 0 AND 100),
    protein_per_100g DOUBLE PRECISION NOT NULL CHECK (protein_per_100g BETWEEN 0 AND 100),
    fat_per_100g DOUBLE PRECISION NOT NULL CHECK (fat_per_100g BETWEEN 0 AND 100),
    updated_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

