package main

import "testing"

func TestValidateProductionCORS(t *testing.T) {
	for _, input := range []string{"", "https://avatar.example.com", "https://avatar.example.com/"} {
		got, err := validateProductionCORS(input, "https://avatar.example.com")
		if err != nil || got != "https://avatar.example.com" {
			t.Fatalf("valid CORS %q rejected: %q, %v", input, got, err)
		}
	}
	for _, input := range []string{"*", "https://evil.example", "https://avatar.example.com/path", "null"} {
		if _, err := validateProductionCORS(input, "https://avatar.example.com"); err == nil {
			t.Fatalf("unsafe CORS %q accepted", input)
		}
	}
}

func TestValidatePublicURL(t *testing.T) {
	if _, err := validatePublicURL("http://avatar.example.com", true); err == nil {
		t.Fatal("production HTTP URL accepted")
	}
	if got, err := validatePublicURL("https://avatar.example.com/", true); err != nil || got != "https://avatar.example.com" {
		t.Fatalf("valid public URL: %q %v", got, err)
	}
}
