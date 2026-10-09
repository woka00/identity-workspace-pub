package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"avatar-id/internal/domain"
)

type authRepoStub struct {
	Repository
	expiresAt time.Time
}

func (r *authRepoStub) CreateSession(_ context.Context, _ int64, _ string, expiresAt time.Time) error {
	r.expiresAt = expiresAt
	return nil
}

func TestSessionUsesSevenDayIdleTimeout(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	repo := &authRepoStub{}
	service := New(repo, nil, func() time.Time { return now })

	session, err := service.createSession(context.Background(), domain.User{ID: 1, Login: "user"})
	if err != nil {
		t.Fatal(err)
	}
	want := now.Add(domain.AuthSessionIdleTimeout)
	if !repo.expiresAt.Equal(want) {
		t.Fatalf("stored expiry=%s, want %s", repo.expiresAt, want)
	}
	if session.ExpiresAt != want.Format(time.RFC3339) {
		t.Fatalf("response expiry=%q, want %q", session.ExpiresAt, want.Format(time.RFC3339))
	}
}

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
	_, err := service.Login(context.Background(), "avatar01", strings.Repeat("x", 129))
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("oversized password error=%v, want unauthorized", err)
	}
}
