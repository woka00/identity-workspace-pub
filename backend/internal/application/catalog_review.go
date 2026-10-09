package application

import (
	"context"
	"fmt"
	"strings"

	"avatar-id/internal/domain"
)

func (s *Service) CatalogCandidates(ctx context.Context, input domain.FoodCatalogCandidateInput) ([]domain.FoodCatalogItem, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.BrandName = strings.TrimSpace(input.BrandName)
	input.NutritionUnit = strings.ToLower(strings.TrimSpace(input.NutritionUnit))
	if input.NutritionUnit == "" {
		input.NutritionUnit = "g"
	}
	if err := validateCatalogFood(domain.FoodCatalogItem{
		Name:                "candidate",
		NutritionUnit:       input.NutritionUnit,
		CaloriesPer100G:     input.CaloriesPer100G,
		CarbohydratePer100G: input.CarbohydratePer100G,
		ProteinPer100G:      input.ProteinPer100G,
		FatPer100G:          input.FatPer100G,
	}); err != nil {
		return nil, err
	}
	return s.repo.FoodCatalogCandidates(ctx, input)
}

func (s *Service) LinkCatalogBarcode(ctx context.Context, barcode, foodID string) (domain.FoodCatalogItem, error) {
	barcode = normalizeCatalogBarcode(barcode)
	if barcode == "" {
		return domain.FoodCatalogItem{}, fmt.Errorf("barcode is required: %w", domain.ErrInvalidInput)
	}
	if err := validateCatalogBarcode(barcode); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	if _, _, err := splitCatalogID(foodID); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	return s.repo.LinkFoodCatalogBarcode(ctx, barcode, foodID)
}

func (s *Service) ReviewUserFoods(ctx context.Context, page int, pendingOnly bool) (domain.FoodCatalogReviewPage, error) {
	if err := RequireAdmin(ctx); err != nil {
		return domain.FoodCatalogReviewPage{}, err
	}
	if page < 0 || page > 10_000 {
		return domain.FoodCatalogReviewPage{}, fmt.Errorf("invalid review page: %w", domain.ErrInvalidInput)
	}
	return s.repo.AdminFoodCatalog(ctx, page, pendingOnly)
}

func (s *Service) PromoteUserFood(ctx context.Context, reviewID int64) (domain.FoodCatalogItem, error) {
	if err := RequireAdmin(ctx); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	if reviewID <= 0 {
		return domain.FoodCatalogItem{}, fmt.Errorf("invalid review id: %w", domain.ErrInvalidInput)
	}
	return s.repo.PromoteFoodCatalogItem(ctx, reviewID)
}

func (s *Service) RejectUserFood(ctx context.Context, reviewID int64) error {
	if err := RequireAdmin(ctx); err != nil {
		return err
	}
	if reviewID <= 0 {
		return fmt.Errorf("invalid review id: %w", domain.ErrInvalidInput)
	}
	return s.repo.RejectFoodCatalogItem(ctx, reviewID)
}

func (s *Service) UpdateUserFoodReview(ctx context.Context, reviewID int64, input domain.FoodCatalogReviewInput) (domain.FoodCatalogReviewItem, error) {
	if err := RequireAdmin(ctx); err != nil {
		return domain.FoodCatalogReviewItem{}, err
	}
	if reviewID <= 0 {
		return domain.FoodCatalogReviewItem{}, fmt.Errorf("invalid review id: %w", domain.ErrInvalidInput)
	}
	input.Barcode = strings.TrimSpace(input.Barcode)
	input.Name = strings.TrimSpace(input.Name)
	input.BrandName = strings.TrimSpace(input.BrandName)
	input.Description = strings.TrimSpace(input.Description)
	if err := validateCatalogFood(domain.FoodCatalogItem{
		Name: input.Name, Barcode: input.Barcode, CaloriesPer100G: input.CaloriesPer100G,
		CarbohydratePer100G: input.CarbohydratePer100G,
		ProteinPer100G:      input.ProteinPer100G, FatPer100G: input.FatPer100G,
	}); err != nil {
		return domain.FoodCatalogReviewItem{}, err
	}
	return s.repo.UpdateAdminFoodCatalog(ctx, reviewID, input)
}
