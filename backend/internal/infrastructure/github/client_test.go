package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"avatar-id/internal/domain"
)

func TestPublicContributionsParsesProfileCalendar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/octocat/contributions" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		if !strings.Contains(r.Header.Get("Accept"), "text/html") || r.Header.Get("User-Agent") == "" || !strings.HasPrefix(r.Header.Get("Accept-Language"), "en-US") {
			t.Fatal("required GitHub headers are missing")
		}
		for day := 1; day <= 7; day++ {
			count, level := day, min(day, 4)
			fmt.Fprintf(w, `<td data-level="%d" id="day-%d" data-date="2026-08-%02d" class="ContributionCalendar-day"></td>`, level, day, day)
			if day == 1 {
				fmt.Fprint(w, `<tool-tip for="day-1">No contributions on August 1st.</tool-tip>`)
			} else {
				fmt.Fprintf(w, `<tool-tip for="day-%d">%d contributions on August %dth.</tool-tip>`, day, count, day)
			}
		}
	}))
	defer server.Close()

	client := Client{HTTPClient: server.Client(), BaseURL: server.URL}
	days, err := client.PublicContributions(context.Background(), "octocat")
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 7 || days[0].Count != 0 || days[6].Date != "2026-08-07" || days[6].Count != 7 || days[6].Level != 4 {
		t.Fatalf("unexpected contribution days: %#v", days)
	}
}

func TestPublicContributionsMapsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()
	client := Client{HTTPClient: server.Client(), BaseURL: server.URL}
	_, err := client.PublicContributions(context.Background(), "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error=%v, want not found", err)
	}
}
