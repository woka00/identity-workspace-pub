package catalogimport

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"avatar-id/internal/domain"
)

func TestPrepareOpenFoodFactsFiltersRussianNutrition(t *testing.T) {
	source := "code\tcountries_tags\tproduct_name_ru\tproduct_name\tbrands\tquantity\tenergy-kcal_100g\tproteins_100g\tfat_100g\tcarbohydrates_100g\n" +
		"4601\ten:russia\tРис\tRice\tМарка\t1 кг\t350\t7\t1\t78\n" +
		"4602\ten:france\tРис\tRice\t\t\t350\t7\t1\t78\n" +
		"4603\tru:russia\tБез КБЖУ\t\t\t\t0\t0\t0\t0\n"
	var out bytes.Buffer
	count, err := PrepareOpenFoodFacts(bytes.NewBufferString(source), &out, false)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Рис")) || bytes.Contains(out.Bytes(), []byte("Без КБЖУ")) {
		t.Fatalf("unexpected output: %s", out.Bytes())
	}
}

func TestCopyPreparedRecords(t *testing.T) {
	input := `{"provider":"open_food_facts","externalId":"1","name":"Рис","servings":[]}` + "\n"
	var got domain.FoodCatalogItem
	count, err := CopyPreparedRecords(bytes.NewBufferString(input), func(item domain.FoodCatalogItem) error {
		got = item
		return nil
	})
	if err != nil || count != 1 || got.Name != "Рис" {
		t.Fatalf("count=%d item=%+v err=%v", count, got, err)
	}
}

func TestPrepareAuchanMergesDuplicatesAndSkipsInvalid(t *testing.T) {
	input := `[
		{"name":"Творог «Марка», 200 г","brand_name":"","calories_per_100g":0,"protein_per_100g":12,"fat_per_100g":0,"carbohydrate_per_100g":3,"external_id":"1","data_quality":{"status":"ok","nutrient_source":"zero_filled"}},
		{"name":"творог Марка, 200 г","brand_name":"Марка","calories_per_100g":120,"protein_per_100g":12,"fat_per_100g":5,"carbohydrate_per_100g":3,"external_id":"2","data_quality":{"status":"ok","nutrient_source":"source"}},
		{"name":"Ошибка","brand_name":"","calories_per_100g":1200,"external_id":"3","data_quality":{"status":"invalid","nutrient_source":"source"}}
	]`
	var output bytes.Buffer
	stats, err := PrepareAuchan(bytes.NewBufferString(input), &output)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Read != 3 || stats.Written != 1 || stats.InvalidSkipped != 1 || stats.DuplicatesMerged != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	var item domain.FoodCatalogItem
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &item); err != nil {
		t.Fatal(err)
	}
	if item.Provider != domain.FoodProviderCommunity || item.BrandName != "Марка" || item.CaloriesPer100G != 120 || item.FatPer100G != 5 || !strings.HasPrefix(item.ExternalID, "auchan-") {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestGzipRoundTrip(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write([]byte("x"))
	_ = writer.Close()
	reader, err := OpenFoodFactsGzip(bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	value, err := io.ReadAll(reader)
	if err != nil || string(value) != "x" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}
