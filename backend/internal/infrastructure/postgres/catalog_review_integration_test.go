package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

func TestBarcodeCandidateLinkRoundTrip(t *testing.T) {
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
	if err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE is_admin=TRUE ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	barcode := fmt.Sprintf("CODE39-TEST-%d", time.Now().UnixNano())
	externalID := fmt.Sprintf("barcode-link-test-%d", time.Now().UnixNano())
	var sourceID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO food_catalog (
			provider, external_id, name, calories_per_100g, carbohydrate_per_100g,
			protein_per_100g, fat_per_100g, servings, data_quality)
		VALUES ('community',$1,'Тестовый продукт',120,16,5,4,'[]'::jsonb,1)
		RETURNING id`, externalID).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(`DELETE FROM user_food_barcode_links WHERE user_id=$1 AND barcode=$2`, userID, barcode)
		_, _ = db.Exec(`DELETE FROM food_catalog WHERE owner_user_id=$1 AND barcode=$2`, userID, barcode)
		_, _ = db.Exec(`DELETE FROM food_catalog WHERE id=$1`, sourceID)
	}()

	userContext := application.WithUser(ctx, domain.User{ID: userID, IsAdmin: true})
	linked, err := repository.LinkFoodCatalogBarcode(userContext, barcode, "community:"+externalID)
	if err != nil {
		t.Fatal(err)
	}
	if linked.Provider != domain.FoodProviderLocal || linked.Barcode != barcode {
		t.Fatalf("linked food = %+v", linked)
	}
	found, err := repository.FoodCatalogByBarcode(userContext, barcode)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != linked.ID || found.Barcode != barcode {
		t.Fatalf("barcode lookup = %+v, linked = %+v", found, linked)
	}
	reviews, err := repository.AdminFoodCatalog(userContext, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	foundReview := false
	var linkedReviewID int64
	for _, review := range reviews.Foods {
		if review.ID == linked.ID && review.Barcode == barcode {
			foundReview = true
			linkedReviewID = review.ReviewID
			break
		}
	}
	if !foundReview {
		t.Fatalf("linked food %q is absent from admin review", linked.ID)
	}
	if err := repository.RejectFoodCatalogItem(userContext, linkedReviewID); err != nil {
		t.Fatal(err)
	}
	reviews, err = repository.AdminFoodCatalog(userContext, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, review := range reviews.Foods {
		if review.ID == linked.ID {
			t.Fatalf("rejected food %q is still present in admin review", linked.ID)
		}
	}
	if retained, err := repository.FoodCatalogByBarcode(userContext, barcode); err != nil || retained.ID != linked.ID {
		t.Fatalf("rejection removed private food: retained=%+v, error=%v", retained, err)
	}
}
