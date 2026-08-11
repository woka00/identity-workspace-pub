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

	"identity-workspace/internal/domain"
)

const (
	requestTokenURL = "https://authentication.fatsecret.com/oauth/request_token"
	authorizeURL    = "https://authentication.fatsecret.com/oauth/authorize"
	accessTokenURL  = "https://authentication.fatsecret.com/oauth/access_token"
	foodEntriesURL  = "https://platform.fatsecret.com/rest/food-entries/v2"
)

type Client struct {
	ConsumerKey    string
	ConsumerSecret string
	HTTPClient     *http.Client
}

type fatSecretFoodEntry struct {
	Meal         string `json:"meal"`
	Calories     string `json:"calories"`
	Carbohydrate string `json:"carbohydrate"`
	Protein      string `json:"protein"`
	Fat          string `json:"fat"`
}

type fatSecretEntriesResponse struct {
	FoodEntries struct {
		FoodEntry []fatSecretFoodEntry `json:"food_entry"`
	} `json:"food_entries"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
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
		meals[name] = &domain.MealNutrition{Meal: name}
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
			meal = &domain.MealNutrition{Meal: mealName}
			meals[mealName] = meal
			mealOrder = append(mealOrder, mealName)
		}
		meal.Calories += calories
		meal.Carbohydrate += carbs
		meal.Protein += protein
		meal.Fat += fat
		meal.EntryCount++
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
	if method == http.MethodPost {
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
	if method == http.MethodPost {
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
