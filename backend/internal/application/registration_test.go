package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type registrationRepoStub struct {
	prepared                              bool
	valid                                 bool
	email, tokenHash, passwordHash, login string
	expires                               time.Time
	completed                             bool
}

func (r *registrationRepoStub) PrepareRegistration(_ context.Context, email, token string, _, expiry time.Time) (bool, error) {
	r.email = email
	r.tokenHash = token
	r.expires = expiry
	return r.prepared, nil
}
func (r *registrationRepoStub) RegistrationTokenValid(_ context.Context, token string, _ time.Time) (bool, error) {
	return r.valid && token == r.tokenHash, nil
}
func (r *registrationRepoStub) CompleteRegistration(_ context.Context, _, login, _, hash string, _ time.Time) error {
	r.completed = true
	r.passwordHash = hash
	r.login = login
	return nil
}

type registrationMailerStub struct {
	email, link string
	err         error
}

func (m *registrationMailerStub) SendVerification(_ context.Context, email, link string) error {
	m.email = email
	m.link = link
	return m.err
}

func TestRegistrationEmailBeforePassword(t *testing.T) {
	now := time.Now()
	repo := &registrationRepoStub{prepared: true, valid: true}
	mailer := &registrationMailerStub{}
	r, err := NewRegistration(repo, mailer, "https://workspace.example")
	if err != nil {
		t.Fatal(err)
	}
	s := New(nil, nil, func() time.Time { return now }).WithRegistration(r)
	if err := s.RequestRegistration(context.Background(), " USER@Example.com "); err != nil {
		t.Fatal(err)
	}
	token := strings.TrimPrefix(mailer.link, "https://workspace.example/#register=")
	if repo.email != "user@example.com" || mailer.email != repo.email || token == repo.tokenHash || len(token) != 43 || repo.tokenHash != sessionTokenHash(token) {
		t.Fatal("email normalization/token storage failed")
	}
	if repo.completed || repo.passwordHash != "" || !repo.expires.Equal(now.Add(30*time.Minute)) {
		t.Fatal("account created before verification or wrong expiry")
	}
	password := "correct horse battery staple"
	if err := s.CompleteRegistration(context.Background(), token, "Person", password); err != nil {
		t.Fatal(err)
	}
	if !repo.completed || repo.passwordHash == password || !verifyPassword(repo.passwordHash, password) {
		t.Fatal("password not securely hashed")
	}
}

func TestRegistrationRejectsInvalidAndExpiredTokens(t *testing.T) {
	repo := &registrationRepoStub{}
	r, _ := NewRegistration(repo, &registrationMailerStub{}, "https://workspace.example")
	s := New(nil, nil, nil).WithRegistration(r)
	for _, token := range []string{"", "short", strings.Repeat("x", 1000), strings.Repeat("a", 43)} {
		if err := s.CompleteRegistration(context.Background(), token, "person", "correct horse battery staple"); !errors.Is(err, ErrRegistrationToken) {
			t.Fatalf("unexpected token result: %v", err)
		}
	}
	if repo.completed {
		t.Fatal("invalid token created account")
	}
}

func TestRegistrationNoSendWhenThrottledAndDisabled(t *testing.T) {
	mailer := &registrationMailerStub{}
	repo := &registrationRepoStub{}
	r, _ := NewRegistration(repo, mailer, "https://workspace.example")
	s := New(nil, nil, nil).WithRegistration(r)
	if err := s.RequestRegistration(context.Background(), "user@example.com"); err != nil {
		t.Fatal(err)
	}
	if mailer.email != "" {
		t.Fatal("sent throttled mail")
	}
	if err := New(nil, nil, nil).RequestRegistration(context.Background(), "user@example.com"); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatal(err)
	}
	for _, email := range []string{"name <user@example.com>", "bad", "user@example.com\r\nBcc: other@example.com", "a@localhost", strings.Repeat("x", 65) + "@example.com"} {
		if _, err := normalizeRegistrationEmail(email); err == nil {
			t.Fatalf("accepted malformed email %q", email)
		}
	}
}

func TestRegistrationConfigurationAndDeliveryFailure(t *testing.T) {
	for _, url := range []string{"http://workspace.example", "https://user:password@workspace.example", "https://workspace.example/?redirect=evil", "https://workspace.example/path"} {
		if _, err := NewRegistration(&registrationRepoStub{}, &registrationMailerStub{}, url); err == nil {
			t.Fatalf("accepted unsafe URL %s", url)
		}
	}
	want := errors.New("provider unavailable")
	mailer := &registrationMailerStub{err: want}
	repo := &registrationRepoStub{prepared: true}
	r, _ := NewRegistration(repo, mailer, "https://workspace.example")
	s := New(nil, nil, nil).WithRegistration(r)
	if err := s.RequestRegistration(context.Background(), "user@example.com"); !errors.Is(err, want) {
		t.Fatal("delivery error was hidden")
	}
	if repo.completed {
		t.Fatal("delivery failure created an account")
	}
}
