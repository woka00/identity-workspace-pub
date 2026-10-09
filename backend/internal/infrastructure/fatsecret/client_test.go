package fatsecret

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"avatar-id/internal/domain"
)

func TestOAuthSignatureRFC5849(t *testing.T) {
	params := url.Values{
		"file": {"vacation.jpg"}, "size": {"original"},
		"oauth_consumer_key": {"dpf43f3p2l4k3l03"}, "oauth_token": {"nnch734d00sl2jdk"},
		"oauth_nonce": {"kllo9940pd9333jh"}, "oauth_timestamp": {"1191242096"},
		"oauth_signature_method": {"HMAC-SHA1"}, "oauth_version": {"1.0"},
	}
	got := oauthSignature(http.MethodGet, "http://photos.example.net/photos", params, "kd94hf93k423kf44", "pfkkdhi9sl3r4s00")
	const want = "tR3+Ty81lMeYAr/Fid0kMTYa/WM="
	if got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestSignedPostRequestUsesFormBody(t *testing.T) {
	client := Client{ConsumerKey: "consumer", ConsumerSecret: "secret"}
	oauth := url.Values{
		"oauth_consumer_key":     {"consumer"},
		"oauth_nonce":            {"nonce"},
		"oauth_signature_method": {"HMAC-SHA1"},
		"oauth_timestamp":        {"1700000000"},
		"oauth_version":          {"1.0"},
		"oauth_callback":         {"https://example.com/callback"},
	}

	request, err := client.signedRequest(
		context.Background(),
		http.MethodPost,
		requestTokenURL,
		nil,
		oauth,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.URL.RawQuery != "" {
		t.Fatalf("POST parameters must not be sent in query: %s", request.URL.RawQuery)
	}
	if request.Header.Get("Authorization") != "" {
		t.Fatal("FatSecret OAuth parameters must not rely on Authorization header")
	}
	if got := request.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", got)
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"oauth_callback", "oauth_consumer_key", "oauth_signature"} {
		if values.Get(name) == "" {
			t.Fatalf("form body is missing %s: %s", name, body)
		}
	}
}

func TestSignedGetRequestUsesQuery(t *testing.T) {
	client := Client{ConsumerKey: "consumer", ConsumerSecret: "secret"}
	oauth := url.Values{
		"oauth_consumer_key":     {"consumer"},
		"oauth_nonce":            {"nonce"},
		"oauth_signature_method": {"HMAC-SHA1"},
		"oauth_timestamp":        {"1700000000"},
		"oauth_version":          {"1.0"},
		"oauth_token":            {"access-token"},
	}
	query := url.Values{"date": {"20000"}, "format": {"json"}}

	request, err := client.signedRequest(
		context.Background(),
		http.MethodGet,
		foodEntriesURL,
		query,
		oauth,
		"access-secret",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := request.URL.Query().Get("date"); got != "20000" {
		t.Fatalf("date = %q", got)
	}
	for _, name := range []string{"oauth_consumer_key", "oauth_signature", "oauth_token"} {
		if request.URL.Query().Get(name) == "" {
			t.Fatalf("query is missing %s: %s", name, request.URL.RawQuery)
		}
	}
	if request.Header.Get("Authorization") != "" {
		t.Fatal("OAuth parameters must be transported as request parameters")
	}
}

func TestOAuthPercentEncodeRFC3986(t *testing.T) {
	const input = "Ladies + Gentlemen / ~"
	const want = "Ladies%20%2B%20Gentlemen%20%2F%20~"
	if got := oauthPercentEncode(input); got != want {
		t.Fatalf("oauthPercentEncode(%q) = %q, want %q", input, got, want)
	}
}

func TestRequestTokenRequiresConfirmedCallback(t *testing.T) {
	client := Client{
		ConsumerKey: "consumer", ConsumerSecret: "secret",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := "oauth_token=request-token&oauth_token_secret=request-secret&oauth_callback_confirmed=false"
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
		})},
	}

	_, _, err := client.RequestToken(context.Background(), "https://example.com/api/integrations/fatsecret/callback")
	if err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("RequestToken error = %v, want callback confirmation error", err)
	}
}

func TestNutritionAggregation(t *testing.T) {
	client := Client{
		ConsumerKey: "consumer", ConsumerSecret: "secret",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Query().Get("date") == "" {
				t.Fatal("date query is missing")
			}
			if request.URL.Query().Get("oauth_signature") == "" {
				t.Fatal("oauth signature is missing from query")
			}
			body := `{"food_entries":{"food_entry":[` +
				`{"food_entry_id":"101","food_id":"4384","serving_id":"16758","food_entry_name":"French toast","food_entry_description":"2 slices","number_of_units":"2","meal":"Breakfast","calories":"317","carbohydrate":"40.04","protein":"11.17","fat":"12.26"},` +
				`{"meal":"Lunch","calories":"500","carbohydrate":"55","protein":"30","fat":"18"}` +
				`]}}`
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
		})},
	}

	nutrition, err := client.Nutrition(context.Background(), "token", "token-secret", "2026-08-04")
	if err != nil {
		t.Fatal(err)
	}
	if nutrition.Calories != 817 || nutrition.EntryCount != 2 {
		t.Fatalf("unexpected totals: %+v", nutrition)
	}
	if nutrition.Protein != 41.2 || nutrition.Fat != 30.3 || nutrition.Carbohydrate != 95 {
		t.Fatalf("unexpected macros: %+v", nutrition)
	}
	if len(nutrition.Meals) != 2 || nutrition.Meals[0].Meal != "Breakfast" || nutrition.Meals[1].Meal != "Lunch" {
		t.Fatalf("unexpected meals: %+v", nutrition.Meals)
	}
	entry := nutrition.Meals[0].Entries[0]
	if entry.ID != "101" || entry.FoodID != "4384" || entry.ServingID != "16758" || entry.Name != "French toast" || entry.NumberOfUnits != 2 {
		t.Fatalf("unexpected detailed entry: %+v", entry)
	}
}

func TestFoodLookupMapsSingleObjects(t *testing.T) {
	client := Client{
		ConsumerKey: "consumer", ConsumerSecret: "secret", Region: "RU", Language: "ru",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Query().Get("oauth_token") != "" {
				t.Fatal("shared food search and details must not use the user's OAuth token")
			}
			if request.URL.Query().Get("region") != "" && (request.URL.Query().Get("region") != "RU" || request.URL.Query().Get("language") != "ru") {
				t.Fatalf("localization missing from query: %s", request.URL.RawQuery)
			}
			var body string
			switch request.URL.Path {
			case "/rest/foods/search/v5":
				if request.URL.Query().Get("max_results") != "20" || request.URL.Query().Get("page_number") != "2" {
					t.Fatalf("unexpected pagination: %s", request.URL.RawQuery)
				}
				body = `{"foods":{"food":{"food_id":"42","food_name":"Овсяная каша","food_type":"Generic","food_description":"100 г - 88 ккал"},"max_results":"20","page_number":"2","total_results":"61"}}`
			case "/rest/food/v5":
				body = `{"food":{"food_id":"42","food_name":"Овсяная каша","food_type":"Generic","servings":{"serving":{"serving_id":"7","serving_description":"100 г","metric_serving_amount":"100","metric_serving_unit":"g","number_of_units":"1","calories":"88","carbohydrate":"15.0","protein":"3.0","fat":"1.7"}}}}`
			default:
				t.Fatalf("unexpected path: %s", request.URL.Path)
			}
			return jsonResponse(request, body), nil
		})},
	}

	searchPage, err := client.SearchFoods(context.Background(), "token", "token-secret", "овсяная каша", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(searchPage.Foods) != 1 || searchPage.Foods[0].ID != "42" || searchPage.Foods[0].Name != "Овсяная каша" || searchPage.Page != 2 || !searchPage.HasMore {
		t.Fatalf("unexpected foods: %+v", searchPage)
	}
	food, err := client.Food(context.Background(), "token", "token-secret", "42")
	if err != nil {
		t.Fatal(err)
	}
	if len(food.Servings) != 1 || food.Servings[0].ID != "7" || food.Servings[0].Calories != 88 {
		t.Fatalf("unexpected food details: %+v", food)
	}
}

func TestFoodSearchAcceptsNumericFoodIDs(t *testing.T) {
	client := Client{ConsumerKey: "consumer", ConsumerSecret: "secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return jsonResponse(request, `{"foods":{"food":{"food_id":12345,"food_name":"Гречка","servings":{"serving":{"serving_id":678,"serving_description":"100 г","number_of_units":"1"}}},"max_results":"20","page_number":"0","total_results":"1"}}`), nil
	})}}
	result, err := client.SearchFoods(context.Background(), "", "", "гречка", 0)
	if err != nil || len(result.Foods) != 1 || result.Foods[0].ID != "12345" {
		t.Fatalf("unexpected numeric food id result: %+v, err=%v", result, err)
	}
}

func TestFoodSearchUsesOnlyRussianCatalog(t *testing.T) {
	requests := 0
	client := Client{
		ConsumerKey: "consumer", ConsumerSecret: "secret", Region: "RU", Language: "ru",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			if request.URL.Query().Get("oauth_token") != "" {
				t.Fatal("Russian catalog search must use an application-signed request")
			}
			if request.URL.Query().Get("region") != "RU" || request.URL.Query().Get("language") != "ru" {
				t.Fatalf("search escaped the Russian catalog: %s", request.URL.RawQuery)
			}
			return jsonResponse(request, `{"foods":{"food":{"food_id":"1","food_name":"Региональный продукт","food_type":"Brand"},"max_results":"20","page_number":"0","total_results":"25"}}`), nil
		})},
	}

	result, err := client.SearchFoods(context.Background(), "token", "token-secret", "product", 0)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || len(result.Foods) != 1 || result.Foods[0].ID != "1" || !result.HasMore {
		t.Fatalf("unexpected Russian search: requests=%d result=%+v", requests, result)
	}
}

func TestFoodDetailsFallBackWithoutLocalizationAndRemainApplicationSigned(t *testing.T) {
	requests := 0
	client := Client{
		ConsumerKey: "consumer", ConsumerSecret: "secret", Region: "RU", Language: "ru",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			if request.URL.Path != "/rest/food/v5" || request.URL.Query().Get("oauth_token") != "" {
				t.Fatalf("food lookup must remain an application-signed v5 request: path=%s query=%s", request.URL.Path, request.URL.RawQuery)
			}
			if request.URL.Query().Get("region") == "RU" {
				return jsonResponse(request, `{"error":{"code":"208","message":"Invalid region: RU"}}`), nil
			}
			return jsonResponse(request, `{"food":{"food_id":"42","food_name":"Product","servings":{"serving":{"serving_id":"7","serving_description":"100 g","number_of_units":"1","calories":"88"}}}}`), nil
		})},
	}

	food, err := client.Food(context.Background(), "token", "token-secret", "42")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || food.ID != "42" || len(food.Servings) != 1 {
		t.Fatalf("unexpected fallback result: requests=%d food=%+v", requests, food)
	}
}

func TestFoodDetailsLegacyFallbackIsApplicationSigned(t *testing.T) {
	requests := 0
	client := Client{
		ConsumerKey: "consumer", ConsumerSecret: "secret", Region: "RU", Language: "ru",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			if request.URL.Query().Get("oauth_token") != "" {
				t.Fatalf("shared food details must not use a profile token: %s", request.URL.RawQuery)
			}
			if request.URL.Path == "/rest/food/v5" {
				return jsonResponse(request, `{"error":{"code":"107","message":"Food is unavailable in v5"}}`), nil
			}
			if request.URL.Path != "/rest/food/v1" {
				t.Fatalf("unexpected legacy fallback path: %s", request.URL.Path)
			}
			return jsonResponse(request, `{"food":{"food_id":"42","food_name":"Legacy product","servings":{"serving":{"serving_id":"7","serving_description":"100 g","number_of_units":"1","calories":"88"}}}}`), nil
		})},
	}

	food, err := client.Food(context.Background(), "token", "token-secret", "42")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || food.ID != "42" || len(food.Servings) != 1 {
		t.Fatalf("unexpected legacy fallback result: requests=%d food=%+v", requests, food)
	}
}

func TestFoodEntryMutationsUseFormBody(t *testing.T) {
	requests := 0
	client := Client{
		ConsumerKey: "consumer", ConsumerSecret: "secret",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			if request.URL.Path != "/rest/food-entries/v1" {
				t.Fatalf("unexpected path: %s", request.URL.Path)
			}
			if request.URL.RawQuery != "" {
				t.Fatalf("mutation parameters leaked into query: %s", request.URL.RawQuery)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatal(err)
			}
			if values.Get("oauth_signature") == "" || values.Get("oauth_token") != "token" || values.Get("format") != "json" {
				t.Fatalf("signed form is incomplete: %s", body)
			}
			switch request.Method {
			case http.MethodPost:
				for key, want := range map[string]string{"food_id": "42", "serving_id": "7", "number_of_units": "1.5", "meal": "breakfast", "date": "20684"} {
					if got := values.Get(key); got != want {
						t.Errorf("POST %s = %q, want %q", key, got, want)
					}
				}
			case http.MethodPut:
				if values.Get("food_entry_id") != "101" || values.Get("meal") != "lunch" {
					t.Fatalf("unexpected PUT body: %s", body)
				}
			case http.MethodDelete:
				if values.Get("food_entry_id") != "101" {
					t.Fatalf("unexpected DELETE body: %s", body)
				}
			default:
				t.Fatalf("unexpected method: %s", request.Method)
			}
			return jsonResponse(request, `{"success":{"value":"1"}}`), nil
		})},
	}

	if err := client.CreateFoodEntry(context.Background(), "token", "token-secret", domain.FoodEntryInput{FoodID: "42", Name: "Каша", ServingID: "7", NumberOfUnits: 1.5, Meal: "breakfast", Date: "2026-08-19"}); err != nil {
		t.Fatal(err)
	}
	if err := client.UpdateFoodEntry(context.Background(), "token", "token-secret", "101", domain.FoodEntryUpdate{Name: "Каша", ServingID: "7", NumberOfUnits: 2, Meal: "lunch"}); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteFoodEntry(context.Background(), "token", "token-secret", "101"); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

func TestAPIErrorAcceptsNumericCode(t *testing.T) {
	client := Client{
		ConsumerKey: "consumer", ConsumerSecret: "secret",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Query().Get("oauth_token") != "" {
				t.Fatal("barcode lookup must not expose a user's OAuth token")
			}
			return jsonResponse(request, `{"error":{"code":211,"message":"No food item detected"}}`), nil
		})},
	}
	_, err := client.BarcodeFood(context.Background(), "4006381333931")
	if err == nil || !strings.Contains(err.Error(), "No food item detected") {
		t.Fatalf("BarcodeFood error = %v, want FatSecret error message", err)
	}
	var coded interface{ ProviderErrorCode() string }
	if !errors.As(err, &coded) || coded.ProviderErrorCode() != "211" {
		t.Fatalf("BarcodeFood error code = %v, want 211", err)
	}
}

func TestBarcodeLookupFallsBackFromRussianRegion(t *testing.T) {
	requests := 0
	client := Client{
		ConsumerKey: "consumer", ConsumerSecret: "secret",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			if request.URL.Query().Get("region") == "RU" {
				if request.URL.Query().Get("language") != "ru" {
					t.Fatal("default Russian language is missing")
				}
				return jsonResponse(request, `{"error":{"code":211,"message":"No food item detected"}}`), nil
			}
			return jsonResponse(request, `{"food":{"food_id":"42","food_name":"Fallback product","food_type":"Brand","servings":{"serving":[{"serving_id":"7","serving_description":"1 serving","number_of_units":"1","calories":"100"}]}}}`), nil
		})},
	}

	food, err := client.BarcodeFood(context.Background(), "4006381333931")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || food.ID != "42" || len(food.Servings) != 1 {
		t.Fatalf("unexpected fallback result: requests=%d food=%+v", requests, food)
	}
}

func jsonResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
