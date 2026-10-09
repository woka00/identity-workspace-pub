package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

func TestGlobusCatalogMigrationMergesExistingGlobalDuplicates(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_TEST_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_TEST_URL is not set")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		CREATE TEMP TABLE food_catalog (
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
			owner_user_id BIGINT,
			UNIQUE(provider, external_id)
		);
		CREATE TEMP TABLE user_nutrition_entries (id BIGSERIAL PRIMARY KEY, food_id BIGINT);
		CREATE TEMP TABLE user_food_barcode_links (
			user_id BIGINT NOT NULL, barcode TEXT NOT NULL, food_id BIGINT NOT NULL,
			PRIMARY KEY (user_id, barcode)
		);
		CREATE TEMP TABLE food_catalog_promotions (
			source_food_id BIGINT PRIMARY KEY, global_food_id BIGINT NOT NULL,
			promoted_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TEMP TABLE food_catalog_review_edits (
			source_food_id BIGINT PRIMARY KEY, updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TEMP TABLE food_catalog_rejections (
			source_food_id BIGINT PRIMARY KEY, rejected_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		t.Fatal(err)
	}

	var canonicalID, duplicateID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO food_catalog (provider, external_id, barcode, name, data_quality)
		VALUES ('open_food_facts','4600000000001','4600000000001','Мандарины с листом, вес',0.9)
		RETURNING id`).Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO food_catalog (
			provider, external_id, name, brand_name, description,
			calories_per_100g, carbohydrate_per_100g, protein_per_100g,
			fat_per_100g, data_quality)
		VALUES (
			'community','existing-duplicate',' Мандарины   с листом, вес ',
			'Проверенный бренд','Полное описание',99,20,3,2,1)
		RETURNING id`).Scan(&duplicateID); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO user_nutrition_entries (food_id) VALUES ($1)`,
		`INSERT INTO user_food_barcode_links (user_id, barcode, food_id) VALUES (1, 'alias', $1)`,
		`INSERT INTO food_catalog_promotions (source_food_id, global_food_id) VALUES (9001, $1)`,
	} {
		if _, err := tx.ExecContext(ctx, query, duplicateID); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`INSERT INTO food_catalog_promotions (source_food_id, global_food_id) VALUES ($1, $1), ($2, $2)`,
		`INSERT INTO food_catalog_review_edits (source_food_id) VALUES ($1), ($2)`,
		`INSERT INTO food_catalog_rejections (source_food_id) VALUES ($1), ($2)`,
	} {
		if _, err := tx.ExecContext(ctx, query, canonicalID, duplicateID); err != nil {
			t.Fatal(err)
		}
	}

	migration, err := migrationsFS.ReadFile("migrations/060_globus_catalog.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}

	var barcode, brand, description string
	var calories, carbohydrate, protein, fat float64
	if err := tx.QueryRowContext(ctx, `
		SELECT barcode, brand_name, description, calories_per_100g,
		       carbohydrate_per_100g, protein_per_100g, fat_per_100g
		FROM food_catalog WHERE id=$1`, canonicalID).Scan(
		&barcode, &brand, &description, &calories, &carbohydrate, &protein, &fat,
	); err != nil {
		t.Fatal(err)
	}
	if barcode != "4600000000001" || brand != "Проверенный бренд" || description != "Полное описание" ||
		calories != 99 || carbohydrate != 20 || protein != 3 || fat != 2 {
		t.Fatalf("canonical product was not merged: barcode=%q brand=%q description=%q macros=%v/%v/%v/%v",
			barcode, brand, description, calories, carbohydrate, protein, fat)
	}

	var duplicateCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM food_catalog WHERE id=$1`, duplicateID).Scan(&duplicateCount); err != nil {
		t.Fatal(err)
	}
	if duplicateCount != 0 {
		t.Fatalf("duplicate product still exists")
	}
	for _, query := range []string{
		`SELECT count(*) FROM user_nutrition_entries WHERE food_id=$1`,
		`SELECT count(*) FROM user_food_barcode_links WHERE food_id=$1`,
	} {
		var count int
		if err := tx.QueryRowContext(ctx, query, canonicalID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("reference was not repointed by %q", query)
		}
	}
	for _, query := range []string{
		`SELECT count(*) FROM food_catalog_promotions WHERE source_food_id=$1`,
		`SELECT count(*) FROM food_catalog_review_edits WHERE source_food_id=$1`,
		`SELECT count(*) FROM food_catalog_rejections WHERE source_food_id=$1`,
	} {
		var count int
		if err := tx.QueryRowContext(ctx, query, canonicalID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("moderation reference was not consolidated by %q", query)
		}
	}

	var importedDuplicateCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM food_catalog
		WHERE external_id='globus-4e4683ed810d5d53ed511e7a0a2e86da'`).Scan(&importedDuplicateCount); err != nil {
		t.Fatal(err)
	}
	if importedDuplicateCount != 0 {
		t.Fatalf("Globus duplicate was inserted despite the existing barcode product")
	}
}
