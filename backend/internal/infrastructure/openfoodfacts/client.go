package openfoodfacts

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"avatar-id/internal/domain"
)

const (
	searchURL       = "https://world.openfoodfacts.org/cgi/search.pl"
	productURL      = "https://world.openfoodfacts.org/api/v2/product/"
	maxResponseSize = 4 << 20
)

type Client struct {
	HTTPClient *http.Client
	UserAgent  string
}

func NewClient(userAgent string) *Client {
	return &Client{HTTPClient: defaultHTTPClient(), UserAgent: userAgent}
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient()
}

func defaultHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second
	transport.IdleConnTimeout = 60 * time.Second
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func decodeResponse(response *http.Response, target any) error {
	limited := io.LimitReader(response.Body, maxResponseSize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(body) > maxResponseSize {
		return errors.New("open food facts response is too large")
	}
	return json.Unmarshal(body, target)
}
func (c Client) request(ctx context.Context, method, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	return c.httpClient().Do(req)
}

type offProduct struct {
	Code          string         `json:"code"`
	ProductName   string         `json:"product_name"`
	ProductNameRU string         `json:"product_name_ru"`
	Brands        string         `json:"brands"`
	Quantity      string         `json:"quantity"`
	Nutriments    map[string]any `json:"nutriments"`
}

func mapProduct(product offProduct) domain.FoodCatalogItem {
	name := strings.TrimSpace(product.ProductNameRU)
	if name == "" {
		name = strings.TrimSpace(product.ProductName)
	}
	calories := first(product.Nutriments, "energy-kcal_100g", "energy-kcal_value")
	item := domain.FoodCatalogItem{Provider: domain.FoodProviderOpenFoodFacts, ExternalID: strings.TrimSpace(product.Code), Barcode: strings.TrimSpace(product.Code), Name: name, BrandName: strings.TrimSpace(product.Brands), Description: strings.TrimSpace(product.Quantity), CaloriesPer100G: calories, CarbohydratePer100G: first(product.Nutriments, "carbohydrates_100g"), ProteinPer100G: first(product.Nutriments, "proteins_100g"), FatPer100G: first(product.Nutriments, "fat_100g"), DataQuality: 0.75}
	item.Servings = []domain.FoodServing{{ID: "g", Description: "г", MetricAmount: 1, MetricUnit: "g", NumberOfUnits: 100, Measurement: "г", Calories: calories, Carbohydrate: item.CarbohydratePer100G, Protein: item.ProteinPer100G, Fat: item.FatPer100G}}
	return item
}

func first(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			switch value := raw.(type) {
			case float64:
				return value
			case json.Number:
				parsed, _ := value.Float64()
				return parsed
			case string:
				parsed, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(value), ",", "."), 64)
				return parsed
			}
		}
	}
	return 0
}

func (c Client) Barcode(ctx context.Context, barcode string) (domain.FoodCatalogItem, error) {
	response, err := c.request(ctx, http.MethodGet, productURL+url.PathEscape(barcode)+".json")
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return domain.FoodCatalogItem{}, fmt.Errorf("open food facts returned %s", response.Status)
	}
	var payload struct {
		Status  int        `json:"status"`
		Product offProduct `json:"product"`
	}
	if err := decodeResponse(response, &payload); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	if payload.Status != 1 || strings.TrimSpace(payload.Product.Code) == "" {
		return domain.FoodCatalogItem{}, domain.ErrNotFound
	}
	return mapProduct(payload.Product), nil
}

func (c Client) Search(ctx context.Context, query string, page int) ([]domain.FoodCatalogItem, bool, error) {
	values := url.Values{"search_terms": {query}, "search_simple": {"1"}, "action": {"process"}, "json": {"1"}, "page_size": {"20"}, "page": {strconv.Itoa(page + 1)}, "countries_tags_en": {"russia"}}
	response, err := c.request(ctx, http.MethodGet, searchURL+"?"+values.Encode())
	if err != nil {
		return nil, false, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, false, fmt.Errorf("open food facts returned %s", response.Status)
	}
	var payload struct {
		Products  []offProduct `json:"products"`
		PageCount int          `json:"page_count"`
		Page      int          `json:"page"`
	}
	if err := decodeResponse(response, &payload); err != nil {
		return nil, false, err
	}
	items := make([]domain.FoodCatalogItem, 0, len(payload.Products))
	for _, product := range payload.Products {
		item := mapProduct(product)
		if item.Name != "" && item.ExternalID != "" {
			items = append(items, item)
		}
	}
	return items, payload.Page < payload.PageCount, nil
}
