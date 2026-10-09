package brevo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVerificationRequestAndRedactedErrors(t *testing.T) {
	for _, status := range []int{201, 400, 401, 429, 500, 302} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, err := New("private-api-key", "sender@example.com", "identity workspace")
			if err != nil {
				t.Fatal(err)
			}
			if c.httpClient.Timeout != 10*time.Second || c.httpClient.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
				t.Fatal("missing timeout/redirect protection")
			}
			c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://api.brevo.com/v3/smtp/email" || r.Method != "POST" || r.Header.Get("api-key") != "private-api-key" {
					t.Fatal("invalid Brevo request")
				}
				var body struct {
					To []struct {
						Email   string `json:"email"`
						Consent bool   `json:"contactPixelTrackingConsent"`
					} `json:"to"`
					Text string `json:"textContent"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body.To) != 1 || body.To[0].Email != "owner@example.com" || body.To[0].Consent || !strings.Contains(body.Text, "https://workspace.example/#register=private-token") {
					t.Fatal("wrong recipient or confirmation link")
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("owner@example.com private-api-key private-token")), Request: r}, nil
			})
			err = c.SendVerification(context.Background(), "owner@example.com", "https://workspace.example/#register=private-token")
			if status == 201 && err != nil {
				t.Fatal(err)
			}
			if status != 201 && (err == nil || strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), "owner@")) {
				t.Fatal("provider failure leaked details or was accepted")
			}
		})
	}
}
