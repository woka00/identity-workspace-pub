package application

import (
	"context"
	"encoding/base64"
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"avatar-id/internal/domain"
)

var (
	ErrRegistrationDisabled = errors.New("регистрация временно недоступна")
	ErrRegistrationToken    = errors.New("ссылка недействительна или истекла; запросите новое письмо")
	ErrRegistrationLogin    = errors.New("этот логин занят; выберите другой")
	ErrRegistrationEmail    = errors.New("почта уже связана с аккаунтом; войдите с вашим логином")
)

const (
	RegistrationTokenTTL          = 30 * time.Minute
	RegistrationEmailCooldown     = time.Minute
	RegistrationEmailHourlyLimit  = 5
	RegistrationGlobalHourlyLimit = 100
	RegistrationGlobalDailyLimit  = 500
)

type RegistrationRepository interface {
	PrepareRegistration(context.Context, string, string, time.Time, time.Time) (bool, error)
	RegistrationTokenValid(context.Context, string, time.Time) (bool, error)
	CompleteRegistration(context.Context, string, string, string, string, time.Time) error
}

type RegistrationMailer interface {
	SendVerification(context.Context, string, string) error
}

type Registration struct {
	repo      RegistrationRepository
	mailer    RegistrationMailer
	publicURL string
}

func NewRegistration(repo RegistrationRepository, mailer RegistrationMailer, publicURL string) (*Registration, error) {
	u, err := url.Parse(publicURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || repo == nil || mailer == nil {
		return nil, errors.New("registration requires a trusted HTTPS PUBLIC_URL, repository and mailer")
	}
	return &Registration{repo: repo, mailer: mailer, publicURL: strings.TrimRight(publicURL, "/")}, nil
}

func (s *Service) WithRegistration(registration *Registration) *Service {
	s.registration = registration
	return s
}

func (s *Service) RegistrationEnabled() bool { return s.registration != nil }

func normalizeRegistrationEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || len(email) < 3 {
		return "", domain.InvalidInputError{Message: "введите корректный адрес почты"}
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || len(parts[0]) > 64 || !strings.Contains(parts[1], ".") {
		return "", domain.InvalidInputError{Message: "введите корректный адрес почты"}
	}
	for _, r := range email {
		if r <= 32 || r >= 127 {
			return "", domain.InvalidInputError{Message: "используйте адрес почты в ASCII-формате"}
		}
	}
	return email, nil
}

func (s *Service) RequestRegistration(ctx context.Context, rawEmail string) error {
	if !s.RegistrationEnabled() {
		return ErrRegistrationDisabled
	}
	email, err := normalizeRegistrationEmail(rawEmail)
	if err != nil {
		return err
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return err
	}
	now := s.now()
	prepared, err := s.registration.repo.PrepareRegistration(ctx, email, tokenHash, now, now.Add(RegistrationTokenTTL))
	if err != nil || !prepared {
		return err
	}
	// The fragment never reaches access logs or Referer headers. A GET does not
	// consume the token: email security scanners cannot activate an account.
	return s.registration.mailer.SendVerification(ctx, email, s.registration.publicURL+"/#register="+token)
}

func (s *Service) CompleteRegistration(ctx context.Context, token, login, password string) error {
	if !s.RegistrationEnabled() {
		return ErrRegistrationDisabled
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != sessionTokenBytes || len(token) != 43 {
		return ErrRegistrationToken
	}
	display, normalized, err := normalizeLogin(login)
	if err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	tokenHash := sessionTokenHash(token)
	valid, err := s.registration.repo.RegistrationTokenValid(ctx, tokenHash, s.now())
	if err != nil {
		return err
	}
	if !valid {
		return ErrRegistrationToken
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return err
	}
	// Storage rechecks expiry and consumes the token in the same transaction as
	// account/profile creation, including concurrent requests and login conflicts.
	return s.registration.repo.CompleteRegistration(ctx, tokenHash, display, normalized, passwordHash, s.now())
}
