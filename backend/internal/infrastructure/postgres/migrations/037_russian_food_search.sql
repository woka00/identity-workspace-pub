-- Keep exact token matching for codes and non-Russian names while allowing
-- Russian product searches to match normal grammatical word forms.
CREATE INDEX IF NOT EXISTS food_catalog_name_search_ru_idx
    ON food_catalog USING gin (to_tsvector('russian', lower(name || ' ' || brand_name)));
