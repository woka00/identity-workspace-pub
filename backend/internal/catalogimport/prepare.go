package catalogimport

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"avatar-id/internal/domain"
)

// PrepareOpenFoodFacts converts the large Open Food Facts TSV/CSV export into
// a compact stream of records understood by the production importer. It never
// keeps the source dataset in memory.
func PrepareOpenFoodFacts(input io.Reader, output io.Writer, includeMissingNutrition bool) (int64, error) {
	reader := csv.NewReader(input)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true
	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("read Open Food Facts header: %w", err)
	}
	columns := make(map[string]int, len(header))
	for index, name := range header {
		columns[strings.TrimSpace(name)] = index
	}
	get := func(row []string, name string) string {
		index, ok := columns[name]
		if !ok || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	encoder := json.NewEncoder(output)
	var written int64
	for {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			// A malformed row should not discard the rest of a multi-gigabyte
			// export. CSV errors contain the line number for diagnostics.
			if _, ok := readErr.(*csv.ParseError); ok {
				continue
			}
			return written, fmt.Errorf("read Open Food Facts row: %w", readErr)
		}
		if !isRussian(get(row, "countries_tags"), get(row, "countries_en"), get(row, "countries")) {
			continue
		}
		barcode := get(row, "code")
		name := firstNonEmpty(get(row, "product_name_ru"), get(row, "product_name"), get(row, "generic_name_ru"), get(row, "generic_name"))
		if barcode == "" || name == "" {
			continue
		}
		calories := number(1000, get(row, "energy-kcal_100g"), get(row, "energy-kcal_value"))
		carbs := number(100, get(row, "carbohydrates_100g"))
		protein := number(100, get(row, "proteins_100g"))
		fat := number(100, get(row, "fat_100g"))
		if !includeMissingNutrition && calories == 0 && carbs == 0 && protein == 0 && fat == 0 {
			continue
		}
		item := domain.FoodCatalogItem{
			Provider: domain.FoodProviderOpenFoodFacts, ExternalID: barcode, Barcode: barcode,
			Name: name, BrandName: get(row, "brands"), Description: get(row, "quantity"),
			CaloriesPer100G: calories, CarbohydratePer100G: carbs, ProteinPer100G: protein, FatPer100G: fat,
			DataQuality: quality(calories, carbs, protein, fat),
			Servings:    []domain.FoodServing{{ID: "g", Description: "г", MetricAmount: 1, MetricUnit: "g", NumberOfUnits: 100, Measurement: "г", Calories: calories, Carbohydrate: carbs, Protein: protein, Fat: fat}},
		}
		if err := encoder.Encode(item); err != nil {
			return written, fmt.Errorf("write prepared catalog: %w", err)
		}
		written++
	}
	return written, nil
}

func isRussian(values ...string) bool {
	for _, value := range values {
		value = strings.ToLower(value)
		if strings.Contains(value, "russia") || strings.Contains(value, "росси") || strings.Contains(value, "ru:russia") || strings.Contains(value, "ru:россия") {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func number(limit float64, values ...string) float64 {
	for _, value := range values {
		value = strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
		if value == "" || value == "null" {
			continue
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil && parsed >= 0 && parsed <= limit {
			return parsed
		}
	}
	return 0
}

func quality(calories, carbs, protein, fat float64) float64 {
	complete := 0
	for _, value := range []float64{calories, carbs, protein, fat} {
		if value > 0 {
			complete++
		}
	}
	return 0.72 + float64(complete)*0.05
}

// CopyPreparedRecords is intentionally small and streaming; it is shared by
// the admin command and tests without exposing database implementation details.
func CopyPreparedRecords(input io.Reader, consume func(domain.FoodCatalogItem) error) (int64, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var count int64
	for scanner.Scan() {
		var item domain.FoodCatalogItem
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return count, fmt.Errorf("decode prepared catalog record %d: %w", count+1, err)
		}
		if err := consume(item); err != nil {
			return count, err
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return count, err
	}
	return count, nil
}

func OpenFoodFactsGzip(input io.Reader) (io.ReadCloser, error) {
	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return nil, err
	}
	return gzipReader, nil
}
