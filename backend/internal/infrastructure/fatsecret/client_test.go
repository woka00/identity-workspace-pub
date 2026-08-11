package fatsecret

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
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
				`{"meal":"Breakfast","calories":"317","carbohydrate":"40.04","protein":"11.17","fat":"12.26"},` +
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
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
