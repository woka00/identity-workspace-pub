package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"avatar-id/internal/domain"
)

func (s *Service) CreateCatalogFood(ctx context.Context, item domain.FoodCatalogItem) (domain.FoodCatalogItem, error) {
	item.Provider = domain.FoodProviderLocal
	item.Name = strings.TrimSpace(item.Name)
	item.BrandName = strings.TrimSpace(item.BrandName)
	item.Description = strings.TrimSpace(item.Description)
	item.Barcode = normalizeCatalogBarcode(item.Barcode)
	item.NutritionUnit = strings.ToLower(strings.TrimSpace(item.NutritionUnit))
	if item.NutritionUnit == "" {
		item.NutritionUnit = "g"
	}
	if err := validateCatalogFood(item); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	// IDs supplied by a client must not be able to target another user's local
	// row. Repeated barcode corrections are resolved by the repository using
	// the authenticated owner and barcode instead.
	item.ExternalID = strconv.FormatInt(s.now().UnixNano(), 10)
	item.DataQuality = 1
	unitLabel := map[string]string{"g": "г", "ml": "мл"}[item.NutritionUnit]
	item.Servings = []domain.FoodServing{{ID: item.NutritionUnit, Description: unitLabel, MetricAmount: 1, MetricUnit: item.NutritionUnit, NumberOfUnits: 100, Measurement: unitLabel, Calories: item.CaloriesPer100G, Carbohydrate: item.CarbohydratePer100G, Protein: item.ProteinPer100G, Fat: item.FatPer100G}}
	if item.PortionAmount > 0 {
		multiplier := item.PortionAmount / 100
		item.Servings = append(item.Servings, domain.FoodServing{
			ID: "portion", Description: fmt.Sprintf("порция (%s %s)", formatCatalogAmount(item.PortionAmount), unitLabel),
			MetricAmount: item.PortionAmount, MetricUnit: item.NutritionUnit, NumberOfUnits: 1, Measurement: "порция",
			Calories: item.CaloriesPer100G * multiplier, Carbohydrate: item.CarbohydratePer100G * multiplier,
			Protein: item.ProteinPer100G * multiplier, Fat: item.FatPer100G * multiplier,
		})
	}
	return s.repo.UpsertFoodCatalog(ctx, item)
}

func formatCatalogAmount(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func normalizeCatalogBarcode(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validateCatalogFood(item domain.FoodCatalogItem) error {
	if item.Name == "" || utf8.RuneCountInString(item.Name) > 300 || utf8.RuneCountInString(item.BrandName) > 300 || utf8.RuneCountInString(item.Description) > 300 {
		return fmt.Errorf("invalid food text fields: %w", domain.ErrInvalidInput)
	}
	if err := validateCatalogBarcode(item.Barcode); err != nil {
		return err
	}
	if item.NutritionUnit != "" && item.NutritionUnit != "g" && item.NutritionUnit != "ml" {
		return fmt.Errorf("invalid food nutrition unit: %w", domain.ErrInvalidInput)
	}
	if math.IsNaN(item.PortionAmount) || math.IsInf(item.PortionAmount, 0) || item.PortionAmount < 0 || (item.PortionAmount > 0 && item.PortionAmount < 0.1) || item.PortionAmount > 10000 {
		return fmt.Errorf("invalid food portion amount: %w", domain.ErrInvalidInput)
	}
	values := []struct {
		name  string
		value float64
		max   float64
	}{
		{"calories", item.CaloriesPer100G, 1000},
		{"carbohydrate", item.CarbohydratePer100G, 100},
		{"protein", item.ProteinPer100G, 100},
		{"fat", item.FatPer100G, 100},
	}
	for _, candidate := range values {
		if math.IsNaN(candidate.value) || math.IsInf(candidate.value, 0) || candidate.value < 0 || candidate.value > candidate.max {
			return fmt.Errorf("invalid food %s: %w", candidate.name, domain.ErrInvalidInput)
		}
	}
	return nil
}

func validateCatalogBarcode(barcode string) error {
	if !utf8.ValidString(barcode) || utf8.RuneCountInString(barcode) > 512 {
		return fmt.Errorf("barcode must contain no more than 512 characters: %w", domain.ErrInvalidInput)
	}
	for _, character := range barcode {
		if unicode.IsControl(character) {
			return fmt.Errorf("barcode must not contain control characters: %w", domain.ErrInvalidInput)
		}
	}
	return nil
}

const (
	foodCatalogPageSize      = 20
	foodCatalogMaximumPage   = 100
	externalFoodSearchBudget = 2500 * time.Millisecond
)

func (s *Service) SearchCatalog(ctx context.Context, query string, exact bool, page int) (domain.FoodCatalogSearchResult, error) {
	query = normalizeFoodQuery(query)
	if len([]rune(query)) < 2 || len([]rune(query)) > 100 {
		return domain.FoodCatalogSearchResult{}, fmt.Errorf("food query must contain 2 to 100 characters: %w", domain.ErrInvalidInput)
	}
	if page < 0 || page > foodCatalogMaximumPage {
		return domain.FoodCatalogSearchResult{}, fmt.Errorf("food search page must be between 0 and %d: %w", foodCatalogMaximumPage, domain.ErrInvalidInput)
	}
	effectiveQuery := query
	if !exact {
		effectiveQuery = correctFoodQuery(query)
	}
	local, localMore, err := s.repo.SearchFoodCatalog(ctx, effectiveQuery, page)
	if err != nil {
		return domain.FoodCatalogSearchResult{}, err
	}
	// A correction is a convenience, never a reason to hide an exact catalogue
	// match for what the user typed.
	if effectiveQuery != query && len(local) == 0 {
		typed, typedMore, typedErr := s.repo.SearchFoodCatalog(ctx, query, page)
		if typedErr != nil {
			return domain.FoodCatalogSearchResult{}, typedErr
		}
		if len(typed) > 0 {
			effectiveQuery, local, localMore = query, typed, typedMore
		}
	}
	items := append([]domain.FoodCatalogItem{}, local...)
	externalUnavailable := false
	if s.foodCatalog != nil && len(local) < foodCatalogPageSize {
		externalCtx, cancel := context.WithTimeout(ctx, externalFoodSearchBudget)
		external, more, externalErr := s.foodCatalog.Search(externalCtx, effectiveQuery, page)
		if (externalErr != nil || len(external) == 0) && effectiveQuery != query && len(local) == 0 {
			external, more, externalErr = s.foodCatalog.Search(externalCtx, query, page)
			if externalErr == nil && len(external) > 0 {
				effectiveQuery = query
			}
		}
		cancel()
		if externalErr == nil {
			// Search is read-only. External cards are persisted only after the user
			// selects one, keeping pagination stable while a query is open.
			items = append(items, external...)
			localMore = localMore || more
		} else if !errors.Is(externalErr, domain.ErrNotFound) {
			externalUnavailable = true
		}
	}
	items = deduplicateCatalog(items)
	result := domain.FoodCatalogSearchResult{
		Query: query, Foods: items, Page: page, HasMore: localMore,
		ExternalUnavailable: externalUnavailable,
	}
	if effectiveQuery != query && len(items) > 0 {
		result.CorrectedQuery = effectiveQuery
	}
	return result, nil
}

func (s *Service) RecentCatalogFoods(ctx context.Context, meal string) ([]domain.FoodCatalogItem, error) {
	meal, err := normalizeFoodMeal(meal, true)
	if err != nil {
		return nil, err
	}
	return s.repo.RecentFoodCatalog(ctx, meal)
}

func (s *Service) CatalogFood(ctx context.Context, id string) (domain.FoodCatalogItem, error) {
	item, err := s.repo.FoodCatalogItem(ctx, id)
	if err == nil {
		if item.Barcode != "" {
			if preferred, preferredErr := s.repo.FoodCatalogByBarcode(ctx, item.Barcode); preferredErr == nil {
				return preferred, nil
			}
		}
		return item, nil
	}
	if !errors.Is(err, domain.ErrNotFound) || s.foodCatalog == nil {
		return domain.FoodCatalogItem{}, err
	}
	provider, externalID, splitErr := splitCatalogID(id)
	if splitErr != nil {
		return domain.FoodCatalogItem{}, splitErr
	}
	if provider != domain.FoodProviderOpenFoodFacts {
		return domain.FoodCatalogItem{}, err
	}
	item, err = s.foodCatalog.Barcode(ctx, externalID)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	saved, err := s.repo.UpsertFoodCatalog(ctx, item)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	if saved.Barcode != "" {
		if preferred, preferredErr := s.repo.FoodCatalogByBarcode(ctx, saved.Barcode); preferredErr == nil {
			return preferred, nil
		}
	}
	return saved, nil
}

func (s *Service) DeleteCatalogFood(ctx context.Context, id string) error {
	provider, externalID, err := splitCatalogID(id)
	if err != nil {
		return err
	}
	if provider != domain.FoodProviderLocal {
		return fmt.Errorf("only personal local foods can be deleted: %w", domain.ErrInvalidInput)
	}
	return s.repo.DeleteFoodCatalog(ctx, externalID)
}

func (s *Service) CatalogBarcode(ctx context.Context, barcode string) (domain.FoodCatalogItem, error) {
	barcode = normalizeCatalogBarcode(barcode)
	if barcode == "" {
		return domain.FoodCatalogItem{}, fmt.Errorf("barcode is required: %w", domain.ErrInvalidInput)
	}
	if err := validateCatalogBarcode(barcode); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	item, err := s.repo.FoodCatalogByBarcode(ctx, barcode)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, domain.ErrNotFound) || s.foodCatalog == nil {
		return domain.FoodCatalogItem{}, err
	}
	item, err = s.foodCatalog.Barcode(ctx, barcode)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	return s.repo.UpsertFoodCatalog(ctx, item)
}

func (s *Service) LocalNutrition(ctx context.Context, date string) (domain.Nutrition, error) {
	date, err := NormalizeDate(date, "nutrition date")
	if err != nil {
		return domain.Nutrition{}, err
	}
	return s.repo.Nutrition(ctx, date)
}
func (s *Service) CreateLocalNutritionEntry(ctx context.Context, input domain.LocalNutritionEntryInput) (domain.Nutrition, error) {
	if err := validateLocalNutritionInput(&input); err != nil {
		return domain.Nutrition{}, err
	}
	return s.repo.CreateNutritionEntry(ctx, input)
}
func (s *Service) UpdateLocalNutritionEntry(ctx context.Context, id string, input domain.LocalNutritionEntryInput) (domain.Nutrition, error) {
	if err := validateLocalNutritionInput(&input); err != nil {
		return domain.Nutrition{}, err
	}
	parsed, err := strconv.ParseInt(strings.TrimPrefix(id, "local-entry:"), 10, 64)
	if err != nil {
		return domain.Nutrition{}, fmt.Errorf("invalid nutrition entry id: %w", domain.ErrInvalidInput)
	}
	return s.repo.UpdateNutritionEntry(ctx, parsed, input)
}
func (s *Service) DeleteLocalNutritionEntry(ctx context.Context, id, date string) (domain.Nutrition, error) {
	parsed, err := strconv.ParseInt(strings.TrimPrefix(id, "local-entry:"), 10, 64)
	if err != nil {
		return domain.Nutrition{}, fmt.Errorf("invalid nutrition entry id: %w", domain.ErrInvalidInput)
	}
	date, err = NormalizeDate(date, "nutrition date")
	if err != nil {
		return domain.Nutrition{}, err
	}
	return s.repo.DeleteNutritionEntry(ctx, parsed, date)
}

func validateLocalNutritionInput(input *domain.LocalNutritionEntryInput) error {
	if _, _, err := splitCatalogID(input.FoodID); err != nil {
		return err
	}
	if strings.TrimSpace(input.ServingID) == "" || input.NumberOfUnits <= 0 || input.NumberOfUnits > 10000 {
		return fmt.Errorf("invalid nutrition amount or serving: %w", domain.ErrInvalidInput)
	}
	meal, err := normalizeFoodMeal(input.Meal, false)
	if err != nil {
		return err
	}
	input.Meal = meal
	input.Date, err = NormalizeDate(input.Date, "nutrition date")
	return err
}

func splitCatalogID(value string) (domain.FoodProvider, string, error) {
	parts := strings.SplitN(strings.TrimSpace(value), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid food id: %w", domain.ErrInvalidInput)
	}
	return domain.FoodProvider(parts[0]), parts[1], nil
}

func deduplicateCatalog(items []domain.FoodCatalogItem) []domain.FoodCatalogItem {
	positions := map[string]int{}
	result := make([]domain.FoodCatalogItem, 0, len(items))
	for _, item := range items {
		key := string(item.Provider) + ":" + item.ExternalID
		if item.Barcode != "" {
			key = "barcode:" + item.Barcode
		}
		item.ID = string(item.Provider) + ":" + item.ExternalID
		if position, exists := positions[key]; exists {
			if catalogItemPriority(item) > catalogItemPriority(result[position]) {
				result[position] = item
			}
		} else {
			positions[key] = len(result)
			result = append(result, item)
		}
	}
	return result
}

func catalogItemPriority(item domain.FoodCatalogItem) float64 {
	if item.Provider == domain.FoodProviderLocal {
		return 2 + item.DataQuality
	}
	return item.DataQuality
}
