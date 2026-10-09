package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"avatar-id/internal/domain"
)

var digitsOnly = regexp.MustCompile(`^[0-9]+$`)

var fatSecretPhraseAliases = map[string]string{
	"вкумно и тоска":  "вкусно и точка",
	"вкумно и точка":  "вкусно и точка",
	"вкусно и тоска":  "вкусно и точка",
	"вкусно точка":    "вкусно и точка",
	"вкусно иточка":   "вкусно и точка",
	"ростикс кфс":     "ростикс kfc",
	"ростикс и кфс":   "ростикс kfc",
	"бургер кингг":    "бургер кинг",
	"макдоналдс":      "макдональдс",
	"макдональдсс":    "макдональдс",
	"овсянная каша":   "овсяная каша",
	"куринная грудка": "куриная грудка",
}

var fatSecretWords = []string{
	"вкусно", "точка", "ростикс", "бургер", "кинг", "макдональдс", "kfc",
	"овсяная", "каша", "банан", "яблоко", "груша", "яйцо", "вареное", "жареное",
	"творог", "йогурт", "молоко", "сыр", "кефир", "куриная", "грудка", "курица",
	"говядина", "свинина", "индейка", "рыба", "лосось", "тунец", "гречка", "рис",
	"макароны", "хлеб", "картофель", "салат", "помидор", "огурец", "кофе", "чай",
	"протеин", "батончик", "шоколад", "печенье", "мороженое", "суп", "борщ", "пицца",
}

func (s *Service) fatSecretConnection(ctx context.Context) (domain.FatSecretConnection, error) {
	if s.fatSecret == nil || !s.fatSecret.Configured() {
		return domain.FatSecretConnection{}, fmt.Errorf("FatSecret is not configured: %w", domain.ErrConflict)
	}
	connection, err := s.repo.FatSecretConnection(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.FatSecretConnection{}, fmt.Errorf("FatSecret account is not connected: %w", domain.ErrConflict)
	}
	return connection, err
}

func (s *Service) SearchFoods(ctx context.Context, query string, exact bool, page int) (domain.FoodSearchResult, error) {
	query = normalizeFoodQuery(query)
	if len([]rune(query)) < 2 || len([]rune(query)) > 100 {
		return domain.FoodSearchResult{}, fmt.Errorf("food query must contain 2 to 100 characters: %w", domain.ErrInvalidInput)
	}
	if page < 0 || page > 1000 {
		return domain.FoodSearchResult{}, fmt.Errorf("food search page must be between 0 and 1000: %w", domain.ErrInvalidInput)
	}
	connection, err := s.fatSecretConnection(ctx)
	if err != nil {
		return domain.FoodSearchResult{}, err
	}
	corrected := query
	if !exact {
		corrected = correctFoodQuery(query)
	}
	searchPage, err := s.fatSecret.SearchFoods(ctx, connection.OAuthToken, connection.OAuthTokenSecret, corrected, page)
	if (err != nil || len(searchPage.Foods) == 0) && corrected != query {
		// Corrections are only a convenience; never let a failed/empty
		// corrected query hide results for the text the user actually entered.
		searchPage, err = s.fatSecret.SearchFoods(ctx, connection.OAuthToken, connection.OAuthTokenSecret, query, page)
		if err != nil {
			return domain.FoodSearchResult{}, err
		}
	} else if err != nil {
		return domain.FoodSearchResult{}, err
	}
	result := domain.FoodSearchResult{Query: query, Foods: searchPage.Foods, Page: searchPage.Page, HasMore: searchPage.HasMore}
	if corrected != query {
		result.CorrectedQuery = corrected
	}
	return result, nil
}

func (s *Service) Food(ctx context.Context, foodID string) (domain.Food, error) {
	if err := validateFatSecretID(foodID, "food"); err != nil {
		return domain.Food{}, err
	}
	connection, err := s.fatSecretConnection(ctx)
	if err != nil {
		return domain.Food{}, err
	}
	return s.fatSecret.Food(ctx, connection.OAuthToken, connection.OAuthTokenSecret, foodID)
}

func (s *Service) BarcodeFood(ctx context.Context, barcode string) (domain.Food, error) {
	barcode, err := normalizeGTIN13(barcode)
	if err != nil {
		return domain.Food{}, err
	}
	if _, err := s.fatSecretConnection(ctx); err != nil {
		return domain.Food{}, err
	}
	return s.fatSecret.BarcodeFood(ctx, barcode)
}

func (s *Service) RecentFoods(ctx context.Context, meal string) ([]domain.Food, error) {
	meal, err := normalizeFoodMeal(meal, true)
	if err != nil {
		return nil, err
	}
	connection, err := s.fatSecretConnection(ctx)
	if err != nil {
		return nil, err
	}
	return s.fatSecret.RecentFoods(ctx, connection.OAuthToken, connection.OAuthTokenSecret, meal)
}

func (s *Service) CreateFoodEntry(ctx context.Context, input domain.FoodEntryInput) (domain.Nutrition, bool, error) {
	if err := validateFoodEntryInput(&input); err != nil {
		return domain.Nutrition{}, false, err
	}
	connection, err := s.fatSecretConnection(ctx)
	if err != nil {
		return domain.Nutrition{}, false, err
	}
	if err := s.fatSecret.CreateFoodEntry(ctx, connection.OAuthToken, connection.OAuthTokenSecret, input); err != nil {
		return domain.Nutrition{}, false, err
	}
	nutrition, err := s.fatSecret.Nutrition(ctx, connection.OAuthToken, connection.OAuthTokenSecret, input.Date)
	if err != nil {
		return domain.Nutrition{Date: input.Date}, true, err
	}
	return nutrition, true, nil
}

func (s *Service) UpdateFoodEntry(ctx context.Context, entryID string, input domain.FoodEntryUpdate, date string) (domain.Nutrition, error) {
	if err := validateFatSecretID(entryID, "food entry"); err != nil {
		return domain.Nutrition{}, err
	}
	date, err := NormalizeDate(date, "nutrition date")
	if err != nil {
		return domain.Nutrition{}, err
	}
	if err := validateFoodEntryUpdate(&input); err != nil {
		return domain.Nutrition{}, err
	}
	connection, err := s.fatSecretConnection(ctx)
	if err != nil {
		return domain.Nutrition{}, err
	}
	if err := s.fatSecret.UpdateFoodEntry(ctx, connection.OAuthToken, connection.OAuthTokenSecret, entryID, input); err != nil {
		return domain.Nutrition{}, err
	}
	return s.fatSecret.Nutrition(ctx, connection.OAuthToken, connection.OAuthTokenSecret, date)
}

func (s *Service) DeleteFoodEntry(ctx context.Context, entryID, date string) (domain.Nutrition, error) {
	if err := validateFatSecretID(entryID, "food entry"); err != nil {
		return domain.Nutrition{}, err
	}
	date, err := NormalizeDate(date, "nutrition date")
	if err != nil {
		return domain.Nutrition{}, err
	}
	connection, err := s.fatSecretConnection(ctx)
	if err != nil {
		return domain.Nutrition{}, err
	}
	if err := s.fatSecret.DeleteFoodEntry(ctx, connection.OAuthToken, connection.OAuthTokenSecret, entryID); err != nil {
		return domain.Nutrition{}, err
	}
	return s.fatSecret.Nutrition(ctx, connection.OAuthToken, connection.OAuthTokenSecret, date)
}

func validateFoodEntryInput(input *domain.FoodEntryInput) error {
	if err := validateFatSecretID(input.FoodID, "food"); err != nil {
		return err
	}
	update := domain.FoodEntryUpdate{
		Name:          input.Name,
		ServingID:     input.ServingID,
		NumberOfUnits: input.NumberOfUnits,
		Meal:          input.Meal,
	}
	if err := validateFoodEntryUpdate(&update); err != nil {
		return err
	}
	input.Name = update.Name
	input.ServingID = update.ServingID
	input.NumberOfUnits = update.NumberOfUnits
	input.Meal = update.Meal
	var err error
	input.Date, err = NormalizeDate(input.Date, "nutrition date")
	return err
}

func validateFoodEntryUpdate(input *domain.FoodEntryUpdate) error {
	if err := validateFatSecretID(input.ServingID, "serving"); err != nil {
		return err
	}
	input.Name = strings.TrimSpace(input.Name)
	if len([]rune(input.Name)) < 1 || len([]rune(input.Name)) > 160 {
		return fmt.Errorf("food name must contain 1 to 160 characters: %w", domain.ErrInvalidInput)
	}
	if input.NumberOfUnits <= 0 || input.NumberOfUnits > 10000 {
		return fmt.Errorf("number of units must be greater than 0 and no more than 10000: %w", domain.ErrInvalidInput)
	}
	meal, err := normalizeFoodMeal(input.Meal, false)
	if err != nil {
		return err
	}
	input.Meal = meal
	return nil
}

func normalizeFoodMeal(meal string, optional bool) (string, error) {
	meal = strings.ToLower(strings.TrimSpace(meal))
	if meal == "" && optional {
		return "", nil
	}
	switch meal {
	case "breakfast", "lunch", "dinner", "other":
		return meal, nil
	default:
		return "", fmt.Errorf("meal must be breakfast, lunch, dinner or other: %w", domain.ErrInvalidInput)
	}
}

func validateFatSecretID(value, label string) error {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 30 || !digitsOnly.MatchString(value) || value == "0" {
		return fmt.Errorf("invalid %s id: %w", label, domain.ErrInvalidInput)
	}
	return nil
}

func normalizeGTIN13(value string) (string, error) {
	value = strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) && r >= '0' && r <= '9' {
			return r
		}
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if !digitsOnly.MatchString(value) || (len(value) != 8 && len(value) != 12 && len(value) != 13) {
		return "", fmt.Errorf("barcode must be EAN-8, UPC-A or EAN-13: %w", domain.ErrInvalidInput)
	}
	value = strings.Repeat("0", 13-len(value)) + value
	if !validGTIN13Checksum(value) {
		return "", fmt.Errorf("barcode checksum is invalid: %w", domain.ErrInvalidInput)
	}
	return value, nil
}

func validGTIN13Checksum(value string) bool {
	if len(value) != 13 {
		return false
	}
	sum := 0
	for index := 0; index < 12; index++ {
		digit := int(value[index] - '0')
		if index%2 == 1 {
			digit *= 3
		}
		sum += digit
	}
	return (10-sum%10)%10 == int(value[12]-'0')
}

func normalizeFoodQuery(value string) string {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "ё", "е"))
	var normalized strings.Builder
	lastSpace := true
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			normalized.WriteRune(char)
			lastSpace = false
			continue
		}
		if !lastSpace {
			normalized.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(normalized.String())
}

func correctFoodQuery(query string) string {
	if alias, ok := fatSecretPhraseAliases[query]; ok {
		return alias
	}
	parts := strings.Fields(query)
	for index, part := range parts {
		if len([]rune(part)) < 4 || digitsOnly.MatchString(part) {
			continue
		}
		best, bestDistance := part, 3
		for _, candidate := range fatSecretWords {
			distance := levenshteinRunes([]rune(part), []rune(candidate))
			if distance < bestDistance {
				best, bestDistance = candidate, distance
			}
		}
		if bestDistance <= 1 {
			parts[index] = best
		}
	}
	corrected := strings.Join(parts, " ")
	if alias, ok := fatSecretPhraseAliases[corrected]; ok {
		return alias
	}
	return corrected
}

func levenshteinRunes(left, right []rune) int {
	previous := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range left {
		current := make([]int, len(right)+1)
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range right {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			current[rightIndex+1] = min(current[rightIndex]+1, previous[rightIndex+1]+1, previous[rightIndex]+cost)
		}
		previous = current
	}
	return previous[len(right)]
}
