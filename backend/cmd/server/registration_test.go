package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRegistrationConfigurationFailsClosed(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "DATA_ENCRYPTION_KEY", "FATSECRET_CONSUMER_KEY", "FATSECRET_CONSUMER_SECRET", "VAPID_PRIVATE_KEY", "BREVO_API_KEY"} {
		t.Setenv(key, "")
		t.Setenv(key+"_FILE", "")
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_URL", "https://workspace.example")
	t.Setenv("REGISTRATION_ENABLED", "false")
	t.Setenv("BREVO_SENDER_EMAIL", "")
	if cfg, err := loadConfig(); err != nil || cfg.registrationEnabled {
		t.Fatalf("disabled config: %v", err)
	}
	t.Setenv("REGISTRATION_ENABLED", "true")
	if _, err := loadConfig(); err == nil {
		t.Fatal("incomplete configuration accepted")
	}
	t.Setenv("DATA_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	t.Setenv("BREVO_API_KEY", "test-api-key")
	t.Setenv("BREVO_SENDER_EMAIL", "sender@example.com")
	if cfg, err := loadConfig(); err != nil || !cfg.registrationEnabled {
		t.Fatalf("valid config: %v", err)
	}
	t.Setenv("PUBLIC_URL", "http://workspace.example")
	if _, err := loadConfig(); err == nil {
		t.Fatal("insecure registration URL accepted")
	}
}
