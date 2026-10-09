package fatsecret

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"avatar-id/internal/domain"
)

const (
	requestTokenURL = "https://authentication.fatsecret.com/oauth/request_token"
	authorizeURL    = "https://authentication.fatsecret.com/oauth/authorize"
	accessTokenURL  = "https://authentication.fatsecret.com/oauth/access_token"
	foodEntriesURL  = "https://platform.fatsecret.com/rest/food-entries/v2"
	foodEntryURL    = "https://platform.fatsecret.com/rest/food-entries/v1"
	foodsSearchURL  = "https://platform.fatsecret.com/rest/foods/search/v5"
	foodURL         = "https://platform.fatsecret.com/rest/food/v5"
	legacyFoodURL   = "https://platform.fatsecret.com/rest/food/v1"
	foodBarcodeURL  = "https://platform.fatsecret.com/rest/food/barcode/find-by-id/v2"
	recentFoodsURL  = "https://platform.fatsecret.com/rest/food/recently-eaten/v2"
)

type Client struct {
	ConsumerKey    string
	ConsumerSecret string
	Region         string
	Language       string
	HTTPClient     *http.Client
}

func NewClient(consumerKey, consumerSecret, region, language string) *Client {
	return &Client{
		ConsumerKey: consumerKey, ConsumerSecret: consumerSecret,
		Region: region, Language: language, HTTPClient: defaultHTTPClient(),
	}
}

type fatSecretFoodEntry struct {
	ID            string `json:"food_entry_id"`
	Description   string `json:"food_entry_description"`
	FoodID        string `json:"food_id"`
	ServingID     string `json:"serving_id"`
	Name          string `json:"food_entry_name"`
	NumberOfUnits string `json:"number_of_units"`
	Meal          string `json:"meal"`
	Calories      string `json:"calories"`
	Carbohydrate  string `json:"carbohydrate"`
	Protein       string `json:"protein"`
	Fat           string `json:"fat"`
}

type fatSecretServing struct {
	ID           flexibleString `json:"serving_id"`
	Description  string         `json:"serving_description"`
	MetricAmount string         `json:"metric_serving_amount"`
	MetricUnit   string         `json:"metric_serving_unit"`
	Units        string         `json:"number_of_units"`
	Measurement  string         `json:"measurement_description"`
	Calories     string         `json:"calories"`
	Carbohydrate string         `json:"carbohydrate"`
	Protein      string         `json:"protein"`
	Fat          string         `json:"fat"`
}

type fatSecretFood struct {
	ID          flexibleString `json:"food_id"`
	Name        string         `json:"food_name"`
	BrandName   string         `json:"brand_name"`
	Type        string         `json:"food_type"`
	Description string         `json:"food_description"`
	ServingID   string         `json:"serving_id"`
	Units       string         `json:"number_of_units"`
	Servings    struct {
		Serving oneOrMany[fatSecretServing] `json:"serving"`
	} `json:"servings"`
}

type flexibleString string

func (v *flexibleString) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*v = flexibleString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*v = flexibleString(n.String())
	return nil
}

type fatSecretAPIError struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
}

type upstreamError struct {
	operation  string
	code       string
	message    string
	httpStatus int
}

func (e *upstreamError) Error() string {
	detail := strings.TrimSpace(e.message)
	if detail == "" {
		detail = http.StatusText(e.httpStatus)
	}
	if e.code != "" {
		return fmt.Sprintf("fatsecret %s (code %s): %s", e.operation, e.code, detail)
	}
	return fmt.Sprintf("fatsecret %s: %s", e.operation, detail)
}

// ProviderErrorCode lets the transport layer map a FatSecret error to a safe,
// useful client message without importing this infrastructure package or
// exposing credentials and request parameters.
func (e *upstreamError) ProviderErrorCode() string { return e.code }

type fatSecretEntriesResponse struct {
	FoodEntries struct {
		FoodEntry []fatSecretFoodEntry `json:"food_entry"`
	} `json:"food_entries"`
	Error *fatSecretAPIError `json:"error,omitempty"`
}

type oneOrMany[T any] []T

func (values *oneOrMany[T]) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		*values = nil
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		return json.Unmarshal(data, (*[]T)(values))
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*values = []T{value}
	return nil
}

func (c Client) AuthorizeURL(token string) string {
	return authorizeURL + "?oauth_token=" + oauthPercentEncode(token)
}

func (c Client) Configured() bool {
	return strings.TrimSpace(c.ConsumerKey) != "" && strings.TrimSpace(c.ConsumerSecret) != ""
}

func (c Client) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient()
}

func defaultHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 15 * time.Second
	transport.IdleConnTimeout = 60 * time.Second
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (c Client) RequestToken(ctx context.Context, callbackURL string) (string, string, error) {
	if !c.Configured() {
		return "", "", errors.New("fatsecret integration is not configured")
	}
	oauth := c.oauthParams("")
	oauth.Set("oauth_callback", callbackURL)
	request, err := c.signedRequest(ctx, http.MethodPost, requestTokenURL, nil, oauth, "")
	if err != nil {
		return "", "", err
	}
	response, err := c.client().Do(request)
	if err != nil {
		return "", "", fmt.Errorf("fatsecret request token: %w", err)
	}
	defer response.Body.Close()
	body, err := readLimitedBody(response.Body, 64_000)
	if err != nil {
		return "", "", err
	}
	values, parseErr := url.ParseQuery(string(body))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", fmt.Errorf("fatsecret request token: %s", fatSecretErrorText(response.Status, body))
	}
	if parseErr != nil {
		return "", "", fmt.Errorf("fatsecret request token response: %w", parseErr)
	}
	token := values.Get("oauth_token")
	secret := values.Get("oauth_token_secret")
	if token == "" || secret == "" {
		return "", "", errors.New("fatsecret request token response is incomplete")
	}
	if !strings.EqualFold(values.Get("oauth_callback_confirmed"), "true") {
		return "", "", errors.New("fatsecret did not confirm the OAuth callback")
	}
	return token, secret, nil
}

func (c Client) AccessToken(ctx context.Context, requestToken, requestSecret, verifier string) (string, string, error) {
	oauth := c.oauthParams(requestToken)
	oauth.Set("oauth_verifier", verifier)
	request, err := c.signedRequest(ctx, http.MethodGet, accessTokenURL, nil, oauth, requestSecret)
	if err != nil {
		return "", "", err
	}
	response, err := c.client().Do(request)
	if err != nil {
		return "", "", fmt.Errorf("fatsecret access token: %w", err)
	}
	defer response.Body.Close()
	body, err := readLimitedBody(response.Body, 64_000)
	if err != nil {
		return "", "", err
	}
	values, parseErr := url.ParseQuery(string(body))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", fmt.Errorf("fatsecret access token: %s", fatSecretErrorText(response.Status, body))
	}
	if parseErr != nil {
		return "", "", fmt.Errorf("fatsecret access token response: %w", parseErr)
	}
	token := values.Get("oauth_token")
	secret := values.Get("oauth_token_secret")
	if token == "" || secret == "" {
		return "", "", errors.New("fatsecret access token response is incomplete")
	}
	return token, secret, nil
}

func (c Client) Nutrition(ctx context.Context, accessToken, accessSecret, date string) (domain.Nutrition, error) {
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return domain.Nutrition{}, errors.New("nutrition date must be YYYY-MM-DD")
	}
	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	dateInt := int(parsedDate.UTC().Sub(epoch) / (24 * time.Hour))

	query := url.Values{}
	query.Set("date", strconv.Itoa(dateInt))
	query.Set("format", "json")
	oauth := c.oauthParams(accessToken)
	request, err := c.signedRequest(ctx, http.MethodGet, foodEntriesURL, query, oauth, accessSecret)
	if err != nil {
		return domain.Nutrition{}, err
	}
	response, err := c.client().Do(request)
	if err != nil {
		return domain.Nutrition{}, fmt.Errorf("fatsecret food diary: %w", err)
	}
	defer response.Body.Close()
	body, err := readLimitedBody(response.Body, 2_000_000)
	if err != nil {
		return domain.Nutrition{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.Nutrition{}, fmt.Errorf("fatsecret food diary: %s", fatSecretErrorText(response.Status, body))
	}

	var payload fatSecretEntriesResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return domain.Nutrition{}, fmt.Errorf("fatsecret food diary response: %w", err)
	}
	if payload.Error != nil {
		return domain.Nutrition{}, fmt.Errorf("fatsecret: %s", strings.TrimSpace(payload.Error.Message))
	}

	result := domain.Nutrition{
		Date:      date,
		Meals:     []domain.MealNutrition{},
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
	mealOrder := []string{"Breakfast", "Lunch", "Dinner", "Other"}
	meals := map[string]*domain.MealNutrition{}
	for _, name := range mealOrder {
		meals[name] = &domain.MealNutrition{Meal: name, Entries: []domain.NutritionEntry{}}
	}
	for _, entry := range payload.FoodEntries.FoodEntry {
		calories := decimalValue(entry.Calories)
		carbs := decimalValue(entry.Carbohydrate)
		protein := decimalValue(entry.Protein)
		fat := decimalValue(entry.Fat)
		result.Calories += calories
		result.Carbohydrate += carbs
		result.Protein += protein
		result.Fat += fat
		result.EntryCount++

		mealName := normalizeFatSecretMeal(entry.Meal)
		meal, ok := meals[mealName]
		if !ok {
			meal = &domain.MealNutrition{Meal: mealName, Entries: []domain.NutritionEntry{}}
			meals[mealName] = meal
			mealOrder = append(mealOrder, mealName)
		}
		meal.Calories += calories
		meal.Carbohydrate += carbs
		meal.Protein += protein
		meal.Fat += fat
		meal.EntryCount++
		meal.Entries = append(meal.Entries, domain.NutritionEntry{
			ID:            entry.ID,
			FoodID:        entry.FoodID,
			ServingID:     entry.ServingID,
			Name:          strings.TrimSpace(entry.Name),
			Description:   strings.TrimSpace(entry.Description),
			Meal:          mealName,
			NumberOfUnits: decimalValue(entry.NumberOfUnits),
			Calories:      roundNutrition(calories),
			Carbohydrate:  roundNutrition(carbs),
			Protein:       roundNutrition(protein),
			Fat:           roundNutrition(fat),
		})
	}
	for _, name := range mealOrder {
		if meal := meals[name]; meal != nil && meal.EntryCount > 0 {
			meal.Calories = roundNutrition(meal.Calories)
			meal.Carbohydrate = roundNutrition(meal.Carbohydrate)
			meal.Protein = roundNutrition(meal.Protein)
			meal.Fat = roundNutrition(meal.Fat)
			result.Meals = append(result.Meals, *meal)
		}
	}
	result.Calories = roundNutrition(result.Calories)
	result.Carbohydrate = roundNutrition(result.Carbohydrate)
	result.Protein = roundNutrition(result.Protein)
	result.Fat = roundNutrition(result.Fat)
	return result, nil
}

func (c Client) SearchFoods(ctx context.Context, _, _ string, queryText string, page int) (domain.FoodSearchPage, error) {
	baseQuery := url.Values{}
	baseQuery.Set("search_expression", queryText)
	baseQuery.Set("max_results", "20")
	// FatSecret uses a zero-based page offset for foods.search.
	baseQuery.Set("page_number", strconv.Itoa(page))
	baseQuery.Set("format", "json")

	// Food search must stay inside the configured catalogue. In production this
	// is the Russian catalogue; falling back to the global/US catalogue returns
	// misleading results (for example, "рис" -> "rice").
	query := c.localizedQueries(baseQuery)[0]
	var payload struct {
		FoodsSearch struct {
			Food         oneOrMany[fatSecretFood] `json:"food"`
			MaxResults   any                      `json:"max_results"`
			PageNumber   any                      `json:"page_number"`
			TotalResults any                      `json:"total_results"`
			Results      struct {
				Food oneOrMany[fatSecretFood] `json:"food"`
			} `json:"results"`
		} `json:"foods_search"`
		LegacyFoods struct {
			Food         oneOrMany[fatSecretFood] `json:"food"`
			MaxResults   any                      `json:"max_results"`
			PageNumber   any                      `json:"page_number"`
			TotalResults any                      `json:"total_results"`
		} `json:"foods"`
	}
	if err := c.doJSON(ctx, http.MethodGet, foodsSearchURL, query, "", "", "food search", &payload); err != nil {
		return domain.FoodSearchPage{}, err
	}
	foods := payload.FoodsSearch.Results.Food
	if len(foods) == 0 {
		foods = payload.FoodsSearch.Food
	}
	if len(foods) == 0 {
		foods = payload.LegacyFoods.Food
	}
	pageNumber := integerValue(payload.FoodsSearch.PageNumber)
	if pageNumber == 0 && page > 0 {
		pageNumber = page
	}
	maxResults := integerValue(payload.FoodsSearch.MaxResults)
	if maxResults <= 0 {
		maxResults = integerValue(payload.LegacyFoods.MaxResults)
	}
	if maxResults <= 0 {
		maxResults = 20
	}
	totalResults := integerValue(payload.FoodsSearch.TotalResults)
	if totalResults == 0 {
		totalResults = integerValue(payload.LegacyFoods.TotalResults)
	}
	mappedFoods := mapFoods(foods)
	hasMore := totalResults > (pageNumber+1)*maxResults
	if totalResults == 0 {
		hasMore = len(mappedFoods) == maxResults
	}
	return domain.FoodSearchPage{Foods: mappedFoods, Page: pageNumber, HasMore: hasMore}, nil
}

func (c Client) Food(ctx context.Context, _, _, foodID string) (domain.Food, error) {
	baseQuery := url.Values{}
	baseQuery.Set("food_id", foodID)
	baseQuery.Set("format", "json")

	type foodRequest struct {
		endpoint string
		query    url.Values
	}
	localized := c.localizedQueries(baseQuery)
	requests := []foodRequest{
		{endpoint: foodURL, query: localized[0]},
		// An exact food ID can come from a user's recently-eaten list. If the
		// Consumer Key cannot use localization, retry the same ID without the
		// Premier-only region parameters. This does not mix search catalogues.
		{endpoint: foodURL, query: localized[1]},
		// v1 is deprecated, but remains a useful compatibility fallback for
		// older food IDs. food.get is a shared call, so it is application-signed
		// as required by FatSecret; a profile access token must not be attached.
		{endpoint: legacyFoodURL, query: localized[0]},
		{endpoint: legacyFoodURL, query: localized[1]},
	}

	var lastErr error
	for _, request := range requests {
		var payload struct {
			Food fatSecretFood `json:"food"`
		}
		if err := c.doJSON(ctx, http.MethodGet, request.endpoint, request.query, "", "", "food details", &payload); err != nil {
			lastErr = err
			continue
		}
		food := mapFood(payload.Food)
		if food.ID != "" {
			return food, nil
		}
		lastErr = errors.New("fatsecret food details response is empty")
	}
	return domain.Food{}, lastErr
}

func (c Client) BarcodeFood(ctx context.Context, barcode string) (domain.Food, error) {
	baseQuery := url.Values{}
	baseQuery.Set("barcode", barcode)
	baseQuery.Set("format", "json")

	var lastErr error
	for _, query := range c.localizedQueries(baseQuery) {
		var payload struct {
			Food fatSecretFood `json:"food"`
		}
		if err := c.doJSON(ctx, http.MethodGet, foodBarcodeURL, query, "", "", "barcode lookup", &payload); err != nil {
			lastErr = err
			continue
		}
		food := mapFood(payload.Food)
		if food.ID != "" {
			return food, nil
		}
		lastErr = errors.New("fatsecret barcode lookup response is empty")
	}
	return domain.Food{}, lastErr
}

func (c Client) RecentFoods(ctx context.Context, accessToken, accessSecret, meal string) ([]domain.Food, error) {
	query := url.Values{}
	query.Set("format", "json")
	if meal != "" {
		query.Set("meal", strings.ToLower(meal))
	}
	var payload struct {
		Foods struct {
			Food oneOrMany[fatSecretFood] `json:"food"`
		} `json:"foods"`
	}
	if err := c.doJSON(ctx, http.MethodGet, recentFoodsURL, query, accessToken, accessSecret, "recent foods", &payload); err != nil {
		return nil, err
	}
	return mapFoods(payload.Foods.Food), nil
}

func (c Client) CreateFoodEntry(ctx context.Context, accessToken, accessSecret string, input domain.FoodEntryInput) error {
	date, err := fatSecretDateInt(input.Date)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("food_id", input.FoodID)
	query.Set("food_entry_name", input.Name)
	query.Set("serving_id", input.ServingID)
	query.Set("number_of_units", strconv.FormatFloat(input.NumberOfUnits, 'f', -1, 64))
	query.Set("meal", strings.ToLower(input.Meal))
	query.Set("date", date)
	query.Set("format", "json")
	return c.doJSON(ctx, http.MethodPost, foodEntryURL, query, accessToken, accessSecret, "create food entry", nil)
}

func (c Client) UpdateFoodEntry(ctx context.Context, accessToken, accessSecret, entryID string, input domain.FoodEntryUpdate) error {
	query := url.Values{}
	query.Set("food_entry_id", entryID)
	query.Set("food_entry_name", input.Name)
	query.Set("serving_id", input.ServingID)
	query.Set("number_of_units", strconv.FormatFloat(input.NumberOfUnits, 'f', -1, 64))
	query.Set("meal", strings.ToLower(input.Meal))
	query.Set("format", "json")
	return c.doJSON(ctx, http.MethodPut, foodEntryURL, query, accessToken, accessSecret, "update food entry", nil)
}

func (c Client) DeleteFoodEntry(ctx context.Context, accessToken, accessSecret, entryID string) error {
	query := url.Values{}
	query.Set("food_entry_id", entryID)
	query.Set("format", "json")
	return c.doJSON(ctx, http.MethodDelete, foodEntryURL, query, accessToken, accessSecret, "delete food entry", nil)
}

func (c Client) localizedQueries(base url.Values) []url.Values {
	region := strings.ToUpper(strings.TrimSpace(c.Region))
	if region == "" {
		region = "RU"
	}
	language := strings.TrimSpace(c.Language)
	if language == "" && region == "RU" {
		language = "ru"
	}
	localized := cloneValues(base)
	localized.Set("region", region)
	if language != "" {
		localized.Set("language", language)
	}
	return []url.Values{localized, cloneValues(base)}
}

func (c Client) doJSON(ctx context.Context, method, endpoint string, query url.Values, accessToken, accessSecret, operation string, target any) error {
	request, err := c.signedRequest(ctx, method, endpoint, query, c.oauthParams(accessToken), accessSecret)
	if err != nil {
		return err
	}
	response, err := c.client().Do(request)
	if err != nil {
		return fmt.Errorf("fatsecret %s: %w", operation, err)
	}
	defer response.Body.Close()
	body, err := readLimitedBody(response.Body, 2_000_000)
	if err != nil {
		return err
	}
	if apiErr := parseUpstreamError(operation, response.StatusCode, body); apiErr != nil {
		return apiErr
	}
	if target == nil || len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("fatsecret %s response: %w", operation, err)
	}
	return nil
}

func parseUpstreamError(operation string, statusCode int, body []byte) error {
	var envelope struct {
		Error *fatSecretAPIError `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		code := ""
		if envelope.Error.Code != nil {
			code = strings.TrimSpace(fmt.Sprint(envelope.Error.Code))
		}
		return &upstreamError{
			operation:  operation,
			code:       code,
			message:    strings.TrimSpace(envelope.Error.Message),
			httpStatus: statusCode,
		}
	}
	if statusCode < 200 || statusCode >= 300 {
		return &upstreamError{
			operation:  operation,
			message:    fatSecretErrorText(http.StatusText(statusCode), body),
			httpStatus: statusCode,
		}
	}
	return nil
}

func mapFoods(source []fatSecretFood) []domain.Food {
	foods := make([]domain.Food, 0, len(source))
	for _, food := range source {
		mapped := mapFood(food)
		if mapped.ID != "" {
			foods = append(foods, mapped)
		}
	}
	return foods
}

func mapFood(source fatSecretFood) domain.Food {
	food := domain.Food{
		ID:          strings.TrimSpace(string(source.ID)),
		Name:        strings.TrimSpace(source.Name),
		BrandName:   strings.TrimSpace(source.BrandName),
		Type:        strings.TrimSpace(source.Type),
		Description: strings.TrimSpace(source.Description),
		ServingID:   strings.TrimSpace(source.ServingID),
		Units:       decimalValue(source.Units),
		Servings:    []domain.FoodServing{},
	}
	for _, serving := range source.Servings.Serving {
		if strings.TrimSpace(string(serving.ID)) == "" || strings.TrimSpace(string(serving.ID)) == "0" {
			continue
		}
		food.Servings = append(food.Servings, domain.FoodServing{
			ID:            strings.TrimSpace(string(serving.ID)),
			Description:   strings.TrimSpace(serving.Description),
			MetricAmount:  decimalValue(serving.MetricAmount),
			MetricUnit:    strings.TrimSpace(serving.MetricUnit),
			NumberOfUnits: decimalValue(serving.Units),
			Measurement:   strings.TrimSpace(serving.Measurement),
			Calories:      roundNutrition(decimalValue(serving.Calories)),
			Carbohydrate:  roundNutrition(decimalValue(serving.Carbohydrate)),
			Protein:       roundNutrition(decimalValue(serving.Protein)),
			Fat:           roundNutrition(decimalValue(serving.Fat)),
		})
	}
	return food
}

func fatSecretDateInt(date string) (string, error) {
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", errors.New("nutrition date must be YYYY-MM-DD")
	}
	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	return strconv.Itoa(int(parsedDate.UTC().Sub(epoch) / (24 * time.Hour))), nil
}

func (c Client) oauthParams(token string) url.Values {
	values := url.Values{}
	values.Set("oauth_consumer_key", c.ConsumerKey)
	values.Set("oauth_nonce", oauthNonce())
	values.Set("oauth_signature_method", "HMAC-SHA1")
	values.Set("oauth_timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	values.Set("oauth_version", "1.0")
	if token != "" {
		values.Set("oauth_token", token)
	}
	return values
}

func (c Client) signedRequest(ctx context.Context, method, endpoint string, query, oauth url.Values, tokenSecret string) (*http.Request, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if query == nil {
		query = url.Values{}
	}

	// FatSecret validates OAuth 1.0 parameters as ordinary request parameters.
	// For POST endpoints we send them as application/x-www-form-urlencoded body;
	// for GET endpoints they are sent in the query string. The signature is
	// calculated over the decoded values before transport encoding.
	signatureParams := cloneValues(query)
	for key, values := range oauth {
		for _, value := range values {
			signatureParams.Add(key, value)
		}
	}

	oauth = cloneValues(oauth)
	oauth.Set("oauth_signature", oauthSignature(
		method,
		parsed.Scheme+"://"+parsed.Host+parsed.Path,
		signatureParams,
		c.ConsumerSecret,
		tokenSecret,
	))

	requestParams := cloneValues(query)
	for key, values := range oauth {
		for _, value := range values {
			requestParams.Add(key, value)
		}
	}

	var body io.Reader
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete {
		parsed.RawQuery = ""
		body = strings.NewReader(requestParams.Encode())
	} else {
		parsed.RawQuery = requestParams.Encode()
	}

	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json, application/x-www-form-urlencoded;q=0.9")
	request.Header.Set("User-Agent", "identity-workspace/1.0")
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return request, nil
}

func oauthSignature(method, endpoint string, params url.Values, consumerSecret, tokenSecret string) string {
	pairs := make([]string, 0, len(params))
	for key, values := range params {
		for _, value := range values {
			pairs = append(pairs, oauthPercentEncode(key)+"="+oauthPercentEncode(value))
		}
	}
	sort.Strings(pairs)
	base := strings.ToUpper(method) + "&" + oauthPercentEncode(endpoint) + "&" + oauthPercentEncode(strings.Join(pairs, "&"))
	key := oauthPercentEncode(consumerSecret) + "&" + oauthPercentEncode(tokenSecret)
	mac := hmac.New(sha1.New, []byte(key))
	_, _ = mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func oauthPercentEncode(value string) string {
	const hexDigits = "0123456789ABCDEF"
	var encoded strings.Builder
	encoded.Grow(len(value))
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '.' || ch == '_' || ch == '~' {
			encoded.WriteByte(ch)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hexDigits[ch>>4])
		encoded.WriteByte(hexDigits[ch&0x0f])
	}
	return encoded.String()
}

func oauthNonce() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func cloneValues(source url.Values) url.Values {
	out := url.Values{}
	for key, values := range source {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func decimalValue(raw string) float64 {
	value, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value
}

func integerValue(raw any) int {
	switch value := raw.(type) {
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := strconv.Atoi(value.String())
		return parsed
	default:
		return 0
	}
}

func roundNutrition(value float64) float64 {
	rounded, _ := strconv.ParseFloat(strconv.FormatFloat(value, 'f', 1, 64), 64)
	return rounded
}

func normalizeFatSecretMeal(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "breakfast":
		return "Breakfast"
	case "lunch":
		return "Lunch"
	case "dinner":
		return "Dinner"
	case "other":
		return "Other"
	default:
		value := strings.TrimSpace(raw)
		if value == "" {
			return "Other"
		}
		return value
	}
}

func fatSecretErrorText(status string, body []byte) string {
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
		return payload.Error.Message
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 240 {
		text = text[:240]
	}
	if text == "" {
		return status
	}
	return status + ": " + text
}

func readLimitedBody(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("fatsecret response is too large")
	}
	return body, nil
}
