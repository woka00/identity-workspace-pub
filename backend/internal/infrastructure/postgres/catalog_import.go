package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"avatar-id/internal/catalogimport"
	"avatar-id/internal/domain"
)

type CatalogImportStats struct {
	Read     int64
	Upserted int64
}

// ImportFoodCatalogJSONL imports a prepared catalog without replacing existing
// rows or touching user nutrition entries.
func (s *Repository) ImportFoodCatalogJSONL(ctx context.Context, input io.Reader) (CatalogImportStats, error) {
	stats := CatalogImportStats{}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("begin catalog import: %w", err)
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO food_catalog (provider, external_id, barcode, name, brand_name, description,
			calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g, servings, data_quality)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (provider, external_id) DO UPDATE SET
			barcode=EXCLUDED.barcode, name=EXCLUDED.name, brand_name=EXCLUDED.brand_name,
			description=EXCLUDED.description, calories_per_100g=EXCLUDED.calories_per_100g,
			carbohydrate_per_100g=EXCLUDED.carbohydrate_per_100g, protein_per_100g=EXCLUDED.protein_per_100g,
			fat_per_100g=EXCLUDED.fat_per_100g, servings=EXCLUDED.servings,
			data_quality=EXCLUDED.data_quality, cached_at=now()`)
	if err != nil {
		return stats, fmt.Errorf("prepare catalog import: %w", err)
	}
	defer statement.Close()

	_, err = catalogimport.CopyPreparedRecords(input, func(item domain.FoodCatalogItem) error {
		stats.Read++
		if validateErr := validateCatalogImportItem(item); validateErr != nil {
			return fmt.Errorf("invalid prepared catalog record %d: %w", stats.Read, validateErr)
		}
		item.Servings = []domain.FoodServing{{ID: "g", Description: "г", MetricAmount: 1, MetricUnit: "g", NumberOfUnits: 100, Measurement: "г", Calories: item.CaloriesPer100G, Carbohydrate: item.CarbohydratePer100G, Protein: item.ProteinPer100G, Fat: item.FatPer100G}}
		servings, marshalErr := jsonMarshal(item.Servings)
		if marshalErr != nil {
			return marshalErr
		}
		if _, execErr := statement.ExecContext(ctx, item.Provider, item.ExternalID, item.Barcode, item.Name, item.BrandName, item.Description,
			item.CaloriesPer100G, item.CarbohydratePer100G, item.ProteinPer100G, item.FatPer100G, servings, item.DataQuality); execErr != nil {
			return execErr
		}
		stats.Upserted++
		return nil
	})
	if err != nil {
		return stats, fmt.Errorf("read catalog import: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit catalog import: %w", err)
	}
	return stats, nil
}

func jsonMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func validateCatalogImportItem(item domain.FoodCatalogItem) error {
	if item.Provider != domain.FoodProviderOpenFoodFacts && item.Provider != domain.FoodProviderCommunity {
		return fmt.Errorf("unsupported provider %q", item.Provider)
	}
	if item.Provider == domain.FoodProviderOpenFoodFacts {
		if item.ExternalID == "" || item.ExternalID != item.Barcode || len(item.Barcode) > 32 {
			return fmt.Errorf("invalid barcode")
		}
		for _, char := range item.Barcode {
			if char < '0' || char > '9' {
				return fmt.Errorf("barcode must contain digits only")
			}
		}
	} else if !strings.HasPrefix(item.ExternalID, "auchan-") || len(item.ExternalID) > 80 || item.Barcode != "" {
		return fmt.Errorf("invalid community catalog identity")
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" || utf8.RuneCountInString(item.Name) > 300 || utf8.RuneCountInString(item.BrandName) > 300 || utf8.RuneCountInString(item.Description) > 300 {
		return fmt.Errorf("invalid text fields")
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
		{"data quality", item.DataQuality, 1},
	}
	for _, candidate := range values {
		if math.IsNaN(candidate.value) || math.IsInf(candidate.value, 0) || candidate.value < 0 || candidate.value > candidate.max {
			return fmt.Errorf("invalid %s", candidate.name)
		}
	}
	return nil
}
