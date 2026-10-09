package application

import (
	"testing"

	"avatar-id/internal/domain"
)

func TestNormalizeWorkLink(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		url       string
		wantTitle string
		wantError bool
	}{
		{name: "https link", title: "Документация", url: "https://example.com/docs", wantTitle: "Документация"},
		{name: "host as title", url: "https://example.com/docs", wantTitle: "example.com"},
		{name: "reject javascript", url: "javascript:alert(1)", wantError: true},
		{name: "reject missing scheme", url: "example.com", wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeWorkLink(domain.WorkLinkInput{Title: tc.title, URL: tc.url})
			if (err != nil) != tc.wantError {
				t.Fatalf("normalizeWorkLink() error = %v, wantError %v", err, tc.wantError)
			}
			if !tc.wantError && got.Title != tc.wantTitle {
				t.Fatalf("normalizeWorkLink() title = %q, want %q", got.Title, tc.wantTitle)
			}
		})
	}
}

func TestNormalizeWorkNote(t *testing.T) {
	got, err := normalizeWorkNote(domain.WorkNoteInput{Title: "  Решения  ", Body: "  Перенести запуск на пятницу.  "})
	if err != nil {
		t.Fatalf("normalizeWorkNote() error = %v", err)
	}
	if got.Title != "Решения" || got.Body != "Перенести запуск на пятницу." {
		t.Fatalf("normalizeWorkNote() = %+v", got)
	}
	if _, err := normalizeWorkNote(domain.WorkNoteInput{Title: "   "}); err == nil {
		t.Fatal("normalizeWorkNote() accepted empty title")
	}
}
