package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"identity-workspace/internal/domain"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(hash, "correct horse battery staple") {
		t.Fatal("valid password was rejected")
	}
	if verifyPassword(hash, "wrong password") {
		t.Fatal("invalid password was accepted")
	}
}

func TestNormalizeLogin(t *testing.T) {
	display, normalized, err := normalizeLogin("  Михаил_01  ")
	if err != nil {
		t.Fatal(err)
	}
	if display != "Михаил_01" || normalized != "михаил_01" {
		t.Fatalf("unexpected login: %q / %q", display, normalized)
	}
	for _, invalid := range []string{"ab", "bad login", "name@host"} {
		if _, _, err := normalizeLogin(invalid); err == nil {
			t.Fatalf("expected invalid login error for %q", invalid)
		}
	}
}

func TestSessionToken(t *testing.T) {
	token, hash, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || len(hash) != 64 || hash != sessionTokenHash(token) {
		t.Fatalf("unexpected token/hash: %q %q", token, hash)
	}
}

func TestLoginRejectsOversizedPasswordBeforeRepository(t *testing.T) {
	service := &Service{}
	_, err := service.Login(context.Background(), "demo-user", strings.Repeat("x", 129))
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("oversized password error=%v, want unauthorized", err)
	}
}
