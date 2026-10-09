package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"avatar-id/internal/domain"
)

func TestFastFoodMigrationGroupsOfficialPortions(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_TEST_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_TEST_URL is not set")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := New(db).Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var raw []byte
	var productCount int
	if err := db.QueryRowContext(ctx, `
		SELECT servings, count(*) OVER ()
		FROM food_catalog
		WHERE provider='community' AND brand_name='ВКУСНО — И ТОЧКА' AND name='Наггетсы'`,
	).Scan(&raw, &productCount); err != nil {
		t.Fatal(err)
	}
	if productCount != 1 {
		t.Fatalf("Наггетсы product count = %d, want 1", productCount)
	}
	var servings []domain.FoodServing
	if err := json.Unmarshal(raw, &servings); err != nil {
		t.Fatal(err)
	}
	if len(servings) != 5 {
		t.Fatalf("servings = %+v, want base unit and four portions", servings)
	}
	expected := []struct {
		description string
		weight      float64
		calories    float64
	}{
		{"4 шт. — 72 г", 72, 177.9984},
		{"6 шт. — 107 г", 107, 264.0011},
		{"9 шт. — 161 г", 161, 396.9938},
		{"18 шт. — 322 г", 322, 793.9876},
	}
	for index, want := range expected {
		serving := servings[index+1]
		if serving.Description != want.description || serving.MetricAmount != want.weight || serving.NumberOfUnits != 1 || math.Abs(serving.Calories-want.calories) > 0.0001 {
			t.Fatalf("serving %d = %+v, want %s / %.0f g / %.4f kcal", index, serving, want.description, want.weight, want.calories)
		}
	}
}
