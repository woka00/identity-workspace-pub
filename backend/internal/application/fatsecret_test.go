package application

import (
	"context"
	"testing"

	"avatar-id/internal/domain"
)

func TestCorrectFoodQuery(t *testing.T) {
	tests := map[string]string{
		"Вкумно и тоска!!!": "вкусно и точка",
		"овсянная каша":     "овсяная каша",
		"куринная грудка":   "куриная грудка",
		"банан":             "банан",
	}
	for input, want := range tests {
		normalized := normalizeFoodQuery(input)
		if got := correctFoodQuery(normalized); got != want {
			t.Errorf("correctFoodQuery(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeGTIN13(t *testing.T) {
	tests := map[string]string{
		"4006381333931": "4006381333931",
		"036000291452":  "0036000291452",
		"96385074":      "0000096385074",
	}
	for input, want := range tests {
		got, err := normalizeGTIN13(input)
		if err != nil {
			t.Fatalf("normalizeGTIN13(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("normalizeGTIN13(%q) = %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"", "123", "4006381333932", "abcdefghijkl"} {
		if _, err := normalizeGTIN13(input); err == nil {
			t.Errorf("normalizeGTIN13(%q) unexpectedly succeeded", input)
		}
	}
}

func TestValidateFoodEntryInput(t *testing.T) {
	input := domain.FoodEntryInput{
		FoodID:        "42",
		Name:          "  Овсяная каша  ",
		ServingID:     "7",
		NumberOfUnits: 1.5,
		Meal:          "BREAKFAST",
		Date:          "2026-08-19",
	}
	if err := validateFoodEntryInput(&input); err != nil {
		t.Fatal(err)
	}
	if input.Name != "Овсяная каша" || input.Meal != "breakfast" {
		t.Fatalf("unexpected normalized input: %#v", input)
	}

	invalid := input
	invalid.Meal = "brunch"
	if err := validateFoodEntryInput(&invalid); err == nil {
		t.Fatal("expected invalid meal error")
	}
	invalid = input
	invalid.NumberOfUnits = 0
	if err := validateFoodEntryInput(&invalid); err == nil {
		t.Fatal("expected invalid units error")
	}
}

func TestSearchPageValidation(t *testing.T) {
	service := &Service{}
	if _, err := service.SearchFoods(context.Background(), "яблоко", false, -1); err == nil {
		t.Fatal("expected negative page to be rejected")
	}
	if _, err := service.SearchFoods(context.Background(), "яблоко", false, 1001); err == nil {
		t.Fatal("expected excessive page to be rejected")
	}
}
