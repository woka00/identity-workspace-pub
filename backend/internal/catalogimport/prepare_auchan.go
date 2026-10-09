package catalogimport

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"avatar-id/internal/domain"
)

type AuchanPrepareStats struct {
	Read             int64
	Written          int64
	InvalidSkipped   int64
	DuplicatesMerged int64
}

type auchanQuality struct {
	Status         string `json:"status"`
	NutrientSource string `json:"nutrient_source"`
	Issue          string `json:"issue"`
}

type auchanProduct struct {
	Name                string        `json:"name"`
	BrandName           string        `json:"brand_name"`
	CaloriesPer100G     float64       `json:"calories_per_100g"`
	CarbohydratePer100G float64       `json:"carbohydrate_per_100g"`
	ProteinPer100G      float64       `json:"protein_per_100g"`
	FatPer100G          float64       `json:"fat_per_100g"`
	ExternalID          string        `json:"external_id"`
	DataQuality         auchanQuality `json:"data_quality"`
}

func normalizeAuchanProductName(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "ё", "е")))
	value = strings.Map(func(char rune) rune {
		switch char {
		case '«', '»', '„', '“', '”', '"', '\'', '`':
			return -1
		}
		if unicode.IsSpace(char) {
			return ' '
		}
		return char
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func auchanProductScore(item auchanProduct) int {
	score := 0
	// Prefer values explicitly present in the source over a formally "ok" row
	// where absent nutrients were replaced with zero.
	if strings.EqualFold(item.DataQuality.NutrientSource, "source") {
		score += 100
	}
	switch strings.ToLower(item.DataQuality.Status) {
	case "ok":
		score += 20
	case "warning":
		score += 10
	}
	for _, value := range []float64{item.CaloriesPer100G, item.ProteinPer100G, item.FatPer100G, item.CarbohydratePer100G} {
		if value > 0 {
			score++
		}
	}
	if strings.TrimSpace(item.BrandName) != "" {
		score++
	}
	return score
}

func mergeAuchanProduct(preferred, alternative auchanProduct) auchanProduct {
	if strings.TrimSpace(preferred.BrandName) == "" {
		preferred.BrandName = alternative.BrandName
	}
	// A zero from a complete source can be a real declared zero. Only supplement
	// zeroes when the selected row explicitly says that missing values were filled.
	if !strings.EqualFold(preferred.DataQuality.NutrientSource, "zero_filled") {
		return preferred
	}
	if preferred.CaloriesPer100G == 0 && alternative.CaloriesPer100G > 0 {
		preferred.CaloriesPer100G = alternative.CaloriesPer100G
	}
	if preferred.ProteinPer100G == 0 && alternative.ProteinPer100G > 0 {
		preferred.ProteinPer100G = alternative.ProteinPer100G
	}
	if preferred.FatPer100G == 0 && alternative.FatPer100G > 0 {
		preferred.FatPer100G = alternative.FatPer100G
	}
	if preferred.CarbohydratePer100G == 0 && alternative.CarbohydratePer100G > 0 {
		preferred.CarbohydratePer100G = alternative.CarbohydratePer100G
	}
	return preferred
}

func auchanDataQuality(item auchanProduct) float64 {
	switch {
	case strings.EqualFold(item.DataQuality.Status, "ok") && strings.EqualFold(item.DataQuality.NutrientSource, "source"):
		return 1
	case strings.EqualFold(item.DataQuality.Status, "ok"):
		return 0.9
	case strings.EqualFold(item.DataQuality.NutrientSource, "source"):
		return 0.82
	default:
		return 0.72
	}
}

func auchanExternalID(name string) string {
	sum := md5.Sum([]byte(strings.ToLower(name))) // Compatibility with migration 038 identifiers; not used for security.
	return "auchan-" + hex.EncodeToString(sum[:])
}

// PrepareAuchan converts the compact JSON array into the global catalog JSONL
// format. Invalid rows are excluded and duplicate names are merged in memory;
// the source currently contains tens of thousands, not millions, of products.
func PrepareAuchan(input io.Reader, output io.Writer) (AuchanPrepareStats, error) {
	stats := AuchanPrepareStats{}
	decoder := json.NewDecoder(input)
	token, err := decoder.Token()
	if err != nil {
		return stats, fmt.Errorf("read Auchan catalog array: %w", err)
	}
	if token != json.Delim('[') {
		return stats, fmt.Errorf("read Auchan catalog array: expected JSON array")
	}
	products := make(map[string]auchanProduct)
	for decoder.More() {
		var item auchanProduct
		if err := decoder.Decode(&item); err != nil {
			return stats, fmt.Errorf("decode Auchan product %d: %w", stats.Read+1, err)
		}
		stats.Read++
		if strings.EqualFold(item.DataQuality.Status, "invalid") || normalizeAuchanProductName(item.Name) == "" {
			stats.InvalidSkipped++
			continue
		}
		key := normalizeAuchanProductName(item.Name)
		current, exists := products[key]
		if !exists {
			products[key] = item
			continue
		}
		stats.DuplicatesMerged++
		if auchanProductScore(item) > auchanProductScore(current) {
			products[key] = mergeAuchanProduct(item, current)
		} else {
			products[key] = mergeAuchanProduct(current, item)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return stats, fmt.Errorf("finish Auchan catalog array: %w", err)
	}

	keys := make([]string, 0, len(products))
	for key := range products {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	encoder := json.NewEncoder(output)
	for _, key := range keys {
		source := products[key]
		item := domain.FoodCatalogItem{
			Provider: domain.FoodProviderCommunity, ExternalID: auchanExternalID(source.Name),
			Name: strings.TrimSpace(source.Name), BrandName: strings.TrimSpace(source.BrandName),
			CaloriesPer100G: source.CaloriesPer100G, CarbohydratePer100G: source.CarbohydratePer100G,
			ProteinPer100G: source.ProteinPer100G, FatPer100G: source.FatPer100G,
			DataQuality: auchanDataQuality(source),
		}
		if err := encoder.Encode(item); err != nil {
			return stats, fmt.Errorf("write Auchan product: %w", err)
		}
		stats.Written++
	}
	return stats, nil
}
