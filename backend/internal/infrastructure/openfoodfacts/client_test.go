package openfoodfacts

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestSearchAcceptsMixedNutrimentTypes(t *testing.T) {
	client := Client{UserAgent: "test/1.0", HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"page":1,"page_count":1,"products":[{"code":"4601","product_name_ru":"Рис","nutriments":{"energy-kcal_100g":"350","proteins_100g":7.1,"fat_100g":null,"carbohydrates_100g":"78,0"}}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	items, _, err := client.Search(context.Background(), "рис", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].CaloriesPer100G != 350 || items[0].CarbohydratePer100G != 78 || items[0].ProteinPer100G != 7.1 {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestDefaultClientIsHardenedAndReused(t *testing.T) {
	client := NewClient("test/1.0")
	if client.httpClient() != client.httpClient() {
		t.Fatal("default HTTP client is recreated")
	}
	request, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err := client.HTTPClient.CheckRedirect(request, nil); err != http.ErrUseLastResponse {
		t.Fatalf("redirect policy error=%v, want ErrUseLastResponse", err)
	}
}

func TestDecodeResponseRejectsOversizedBody(t *testing.T) {
	response := &http.Response{Body: io.NopCloser(bytes.NewReader(make([]byte, maxResponseSize+1)))}
	if err := decodeResponse(response, &struct{}{}); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized response error=%v", err)
	}
}
