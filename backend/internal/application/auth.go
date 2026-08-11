package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"identity-workspace/internal/domain"
)

const (
	passwordIterations = 600000
	passwordSaltBytes  = 16
	passwordKeyBytes   = 32
	sessionTokenBytes  = 32
	sessionLifetime    = 30 * 24 * time.Hour
)

func normalizeLogin(raw string) (string, string, error) {
	login := strings.TrimSpace(raw)
	if len([]rune(login)) < 3 || len([]rune(login)) > 32 {
		return "", "", domain.InvalidInputError{Message: "логин должен содержать от 3 до 32 символов"}
	}
	for _, r := range login {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			continue
		}
		return "", "", domain.InvalidInputError{Message: "логин может содержать буквы, цифры, точку, дефис и подчёркивание"}
	}
	return login, strings.ToLower(login), nil
}

// NormalizeLoginForAdmin validates a login and returns its normalized database form.
func NormalizeLoginForAdmin(raw string) (string, error) {
	_, normalized, err := normalizeLogin(raw)
	return normalized, err
}

func validatePassword(password string) error {
	length := len([]rune(password))
	if length < 15 || length > 128 {
		return domain.InvalidInputError{Message: "пароль должен содержать от 15 до 128 символов"}
	}
	return nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := pbkdf2SHA256([]byte(password), salt, passwordIterations, passwordKeyBytes)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s",
		passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 10000 || iterations > 1000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 8 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) < 16 {
		return false
	}
	actual := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLength int) []byte {
	hashLength := sha256.Size
	blocks := (keyLength + hashLength - 1) / hashLength
	derived := make([]byte, 0, blocks*hashLength)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		derived = append(derived, t...)
	}
	return derived[:keyLength]
}

func passwordHashNeedsUpgrade(encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return true
	}
	iterations, err := strconv.Atoi(parts[1])
	return err != nil || iterations < passwordIterations
}

var dummyHashOnce = struct {
	sync.Once
	value string
}{}

func dummyPasswordHash() string {
	dummyHashOnce.Do(func() {
		salt := []byte("identity-dummy-v1")
		key := pbkdf2SHA256([]byte("not-a-real-password"), salt, passwordIterations, passwordKeyBytes)
		dummyHashOnce.value = fmt.Sprintf("pbkdf2-sha256$%d$%s$%s",
			passwordIterations,
			base64.RawStdEncoding.EncodeToString(salt),
			base64.RawStdEncoding.EncodeToString(key),
		)
	})
	return dummyHashOnce.value
}

// HashPasswordForAdmin validates and hashes a password for the built-in admin CLI.
func HashPasswordForAdmin(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	return hashPassword(password)
}

func newSessionToken() (string, string, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, sessionTokenHash(token), nil
}

func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Login(ctx context.Context, login, password string) (domain.AuthSession, error) {
	// Bound attacker-controlled work before database access and password hashing.
	// Existing preview passwords may be shorter than the production policy, so
	// login enforces only the maximum; the admin CLI enforces both bounds.
	if len([]rune(password)) > 128 {
		return domain.AuthSession{}, domain.ErrUnauthorized
	}
	_, normalized, err := normalizeLogin(login)
	if err != nil {
		return domain.AuthSession{}, domain.ErrUnauthorized
	}
	credential, err := s.repo.UserByLogin(ctx, normalized)
	if errors.Is(err, domain.ErrNotFound) {
		// Выполняем сопоставимую по стоимости проверку, чтобы отсутствие логина
		// не определялось по времени ответа.
		_ = verifyPassword(dummyPasswordHash(), password)
		return domain.AuthSession{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.AuthSession{}, err
	}
	if !verifyPassword(credential.PasswordHash, password) {
		return domain.AuthSession{}, domain.ErrUnauthorized
	}
	if passwordHashNeedsUpgrade(credential.PasswordHash) {
		upgraded, hashErr := hashPassword(password)
		if hashErr != nil {
			return domain.AuthSession{}, hashErr
		}
		if err := s.repo.UpdatePasswordHash(ctx, credential.User.ID, upgraded); err != nil {
			return domain.AuthSession{}, err
		}
	}
	return s.createSession(ctx, credential.User)
}

func (s *Service) createSession(ctx context.Context, user domain.User) (domain.AuthSession, error) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return domain.AuthSession{}, err
	}
	expires := s.now().Add(sessionLifetime)
	if err := s.repo.CreateSession(ctx, user.ID, tokenHash, expires); err != nil {
		return domain.AuthSession{}, err
	}
	return domain.AuthSession{User: user, Token: token, ExpiresAt: expires.UTC().Format(time.RFC3339)}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (domain.User, error) {
	if strings.TrimSpace(token) == "" {
		return domain.User{}, domain.ErrUnauthorized
	}
	return s.repo.UserBySession(ctx, sessionTokenHash(token), s.now())
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return s.repo.DeleteSession(ctx, sessionTokenHash(token))
}
