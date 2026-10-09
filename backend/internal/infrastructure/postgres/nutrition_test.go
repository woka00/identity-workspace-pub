package postgres

import (
	"context"
	"database/sql"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

func TestCreateNutritionEntryQueryPlaceholders(t *testing.T) {
	matches := regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(createNutritionEntryQuery, -1)
	if len(matches) == 0 {
		t.Fatal("create nutrition entry query has no placeholders")
	}
	maximum := 0
	for _, match := range matches {
		value, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		if value > maximum {
			maximum = value
		}
	}
	input := domain.LocalNutritionEntryInput{Date: "2026-08-23", Meal: "breakfast", NumberOfUnits: 250}
	item := domain.FoodCatalogItem{Name: "Pineapple juice", Provider: domain.FoodProviderOpenFoodFacts, ExternalID: "4600000000000"}
	serving := domain.FoodServing{ID: "g", Description: "g"}
	args := createNutritionEntryArgs(1, input, item, serving, 125, 30, 1, 0)
	if maximum != len(args) {
		t.Fatalf("create nutrition entry query expects %d arguments, got %d", maximum, len(args))
	}
	if args[10] != serving.ID || args[11] != item.Provider || args[12] != item.ExternalID {
		t.Fatalf("serving and product identity arguments are misplaced: %#v", args[10:])
	}
}

func TestValidateCatalogImportItemAllowsPreparedAuchanProducts(t *testing.T) {
	item := domain.FoodCatalogItem{
		Provider: domain.FoodProviderCommunity, ExternalID: "auchan-0123456789abcdef",
		Name: "Котлеты", CaloriesPer100G: 205, ProteinPer100G: 12,
		FatPer100G: 13, CarbohydratePer100G: 10, DataQuality: 1,
	}
	if err := validateCatalogImportItem(item); err != nil {
		t.Fatalf("valid prepared Auchan product was rejected: %v", err)
	}
	item.ExternalID = "other-source-1"
	if err := validateCatalogImportItem(item); err == nil {
		t.Fatal("unexpected community source was accepted")
	}
}

func TestSearchFoodCatalogRanksExactWordAboveInflectedMatch(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_TEST_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_TEST_URL is not set")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository := New(db)
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var userID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	marker := "ранктест" + suffix
	exactID := "search-exact-" + suffix
	inflectedID := "search-inflected-" + suffix
	defer func() {
		_, _ = db.Exec(`DELETE FROM food_catalog WHERE provider='local' AND owner_user_id=$1 AND external_id IN ($2,$3)`, userID, exactID, inflectedID)
	}()
	for _, item := range []struct {
		externalID string
		name       string
		quality    float64
	}{
		{exactID, "Сыр " + marker + " мягкий", 0.1},
		{inflectedID, "Блинчики сытные с сыром " + marker, 1},
	} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO food_catalog (provider, external_id, owner_user_id, name, servings, data_quality)
			VALUES ('local',$1,$2,$3,'[]'::jsonb,$4)`, item.externalID, userID, item.name, item.quality); err != nil {
			t.Fatal(err)
		}
	}

	items, _, err := repository.SearchFoodCatalog(application.WithUser(ctx, domain.User{ID: userID}), "сыр "+marker, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 2 {
		t.Fatalf("search returned %d items, want both regression fixtures: %+v", len(items), items)
	}
	if items[0].ExternalID != exactID || items[1].ExternalID != inflectedID {
		t.Fatalf("search order = %q, %q; want exact word before inflected match", items[0].Name, items[1].Name)
	}
	if items[0].Name != "Сыр "+marker+" мягкий" {
		t.Fatalf("unexpected exact result: %q", items[0].Name)
	}
}

func TestSearchFoodCatalogSupportsPrefixTypoBarcodeAndRecentBoost(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_TEST_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_TEST_URL is not set")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository := New(db)
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var userID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	prefixName := "Маркерпродукт" + suffix
	typoMarker := "опечаткатест" + suffix
	usageName := "Йогурт " + "использованиетест" + suffix
	punctuationName := "Кефир 2,5% " + suffix
	barcode := "29" + suffix[len(suffix)-11:]
	type insertedFood struct {
		externalID string
		name       string
		barcode    string
		quality    float64
	}
	fixtures := []insertedFood{
		{"prefix-" + suffix, prefixName, "", 0.5},
		{"typo-" + suffix, "Карамельный " + typoMarker, "", 0.5},
		{"barcode-" + suffix, "Совсем другое название " + suffix, barcode, 0.5},
		{"recent-" + suffix, usageName, "", 0.1},
		{"unused-" + suffix, usageName, "", 1},
		{"punctuation-" + suffix, punctuationName, "", 0.5},
	}
	ids := make(map[string]int64, len(fixtures))
	for _, fixture := range fixtures {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO food_catalog (provider, external_id, owner_user_id, barcode, name, servings, data_quality)
			VALUES ('local',$1,$2,$3,$4,'[]'::jsonb,$5)
			RETURNING id`, fixture.externalID, userID, fixture.barcode, fixture.name, fixture.quality).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[fixture.externalID] = id
	}
	defer func() {
		_, _ = db.Exec(`DELETE FROM user_nutrition_entries WHERE user_id=$1 AND food_id=$2`, userID, ids["recent-"+suffix])
		for _, fixture := range fixtures {
			_, _ = db.Exec(`DELETE FROM food_catalog WHERE id=$1`, ids[fixture.externalID])
		}
	}()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_nutrition_entries
		(user_id, entry_date, meal, food_id, name, number_of_units, serving_id)
		VALUES ($1, CURRENT_DATE, 'breakfast', $2, $3, 1, 'g')`, userID, ids["recent-"+suffix], usageName); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		query     string
		wantFirst string
	}{
		{"prefix", prefixName[:len(prefixName)-4], "prefix-" + suffix},
		{"typo", "карамелный " + typoMarker, "typo-" + suffix},
		{"barcode", barcode, "barcode-" + suffix},
		{"recent", usageName, "recent-" + suffix},
		{"punctuation", "2,5% " + suffix, "punctuation-" + suffix},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			items, _, err := repository.SearchFoodCatalog(application.WithUser(ctx, domain.User{ID: userID}), test.query, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) == 0 || items[0].ExternalID != test.wantFirst {
				t.Fatalf("search %q returned %+v, want %q first", test.query, items, test.wantFirst)
			}
		})
	}
}
