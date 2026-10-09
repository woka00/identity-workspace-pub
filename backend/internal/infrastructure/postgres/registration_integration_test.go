package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

func registrationTestRepository(t *testing.T) *Repository {
	t.Helper()
	dsn := os.Getenv("DATABASE_TEST_URL")
	if dsn == "" {
		t.Skip("DATABASE_TEST_URL is not set")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("registration_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); admin.Close() })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cipher, err := NewSecretCipher(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	r := New(db, cipher)
	if err := r.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}

type verificationCapture struct{ link string }

func (m *verificationCapture) SendVerification(_ context.Context, _, link string) error {
	m.link = link
	return nil
}
func verificationHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TestRegistrationDatabaseLifecycle(t *testing.T) {
	r := registrationTestRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	mailer := &verificationCapture{}
	registration, err := application.NewRegistration(r, mailer, "https://workspace.example")
	if err != nil {
		t.Fatal(err)
	}
	s := application.New(r, nil, func() time.Time { return now }).WithRegistration(registration)
	if err := s.RequestRegistration(ctx, "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	token := strings.TrimPrefix(mailer.link, "https://workspace.example/#register=")
	hash := verificationHash(token)
	var enc, index, stored string
	if err := r.db.QueryRow(`SELECT email_encrypted,email_hash,token_hash FROM registration_requests`).Scan(&enc, &index, &stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(enc, "owner@example.com") || enc == "owner@example.com" || len(index) != 64 || stored != hash || stored == token {
		t.Fatal("raw registration secrets persisted")
	}
	if _, err := s.Login(ctx, "newperson", "correct horse battery staple"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("login allowed before registration")
	}
	if err := s.CompleteRegistration(ctx, token, "avatar01", "correct horse battery staple"); !errors.Is(err, application.ErrRegistrationLogin) {
		t.Fatalf("login conflict: %v", err)
	}
	if valid, _ := r.RegistrationTokenValid(ctx, hash, now); !valid {
		t.Fatal("login conflict consumed token")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.CompleteRegistration(ctx, token, "newperson", "correct horse battery staple")
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, application.ErrRegistrationToken) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("created %d accounts", success)
	}
	if err := s.CompleteRegistration(ctx, token, "otherperson", "correct horse battery staple"); !errors.Is(err, application.ErrRegistrationToken) {
		t.Fatal("token replay accepted")
	}
	session, err := s.Login(ctx, "newperson", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if session.User.IsAdmin {
		t.Fatal("registered admin")
	}
	var confirmed bool
	if err := r.db.QueryRow(`SELECT email_encrypted,email_verified_at IS NOT NULL FROM users WHERE id=$1`, session.User.ID).Scan(&enc, &confirmed); err != nil {
		t.Fatal(err)
	}
	plain, err := r.secretCipher.DecryptFor(userEmailPurpose(session.User.ID), enc)
	if err != nil || plain != "owner@example.com" || !confirmed {
		t.Fatal("email not encrypted/bound/verified")
	}
	if _, err := r.secretCipher.DecryptFor(userEmailPurpose(session.User.ID+1), enc); err == nil {
		t.Fatal("cross-account ciphertext accepted")
	}
	userCtx := application.WithUser(ctx, session.User)
	if _, err := s.State(userCtx); err != nil {
		t.Fatalf("new account state unavailable: %v", err)
	}
	if _, err := r.Trackers(userCtx); err != nil {
		t.Fatalf("new account trackers unavailable: %v", err)
	}
	if categories, err := r.TaskCategories(userCtx); err != nil || len(categories) != 2 {
		t.Fatalf("new account categories unavailable: %v", err)
	}
	if _, err := r.WorkspacePreferences(userCtx); err != nil {
		t.Fatalf("new account profile absent: %v", err)
	}
	var settings int
	if err := r.db.QueryRow(`SELECT count(*) FROM user_tracker_settings WHERE user_id=$1`, session.User.ID).Scan(&settings); err != nil || settings != 1 {
		t.Fatal("new account settings absent")
	}
	// Existing addresses receive the same request flow. Only a holder of a valid
	// email token can discover that the address is already associated with a user.
	now = now.Add(2 * time.Minute)
	if err := s.RequestRegistration(ctx, "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	duplicate := strings.TrimPrefix(mailer.link, "https://workspace.example/#register=")
	if err := s.CompleteRegistration(ctx, duplicate, "otherperson", "correct horse battery staple"); !errors.Is(err, application.ErrRegistrationEmail) {
		t.Fatal("duplicate email accepted")
	}
}

func TestSessionTouchIsThrottledAndAbsoluteLifetimeIsEnforced(t *testing.T) {
	r := registrationTestRepository(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)
	tokenHash := strings.Repeat("a", 64)
	if _, err := r.db.Exec(`UPDATE users SET is_enabled=TRUE WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`
		INSERT INTO auth_sessions(user_id,token_hash,expires_at,created_at,last_seen_at)
		VALUES(1,$1,$2,$3,$3)`, tokenHash, base.Add(domain.AuthSessionIdleTimeout), base); err != nil {
		t.Fatal(err)
	}
	if _, err := r.UserBySession(ctx, tokenHash, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var lastSeen time.Time
	if err := r.db.QueryRow(`SELECT last_seen_at FROM auth_sessions WHERE token_hash=$1`, tokenHash).Scan(&lastSeen); err != nil {
		t.Fatal(err)
	}
	if !lastSeen.Equal(base) {
		t.Fatalf("session was touched too early: got %s want %s", lastSeen, base)
	}
	touchedAt := base.Add(domain.AuthSessionTouchInterval + time.Minute)
	if _, err := r.UserBySession(ctx, tokenHash, touchedAt); err != nil {
		t.Fatal(err)
	}
	if err := r.db.QueryRow(`SELECT last_seen_at FROM auth_sessions WHERE token_hash=$1`, tokenHash).Scan(&lastSeen); err != nil {
		t.Fatal(err)
	}
	if !lastSeen.Equal(touchedAt) {
		t.Fatalf("session touch=%s, want %s", lastSeen, touchedAt)
	}
	if _, err := r.db.Exec(`UPDATE auth_sessions SET created_at=$2,last_seen_at=$3,expires_at=$4 WHERE token_hash=$1`, tokenHash, base.Add(-domain.AuthSessionAbsoluteTimeout-time.Minute), base, base.Add(domain.AuthSessionIdleTimeout)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.UserBySession(ctx, tokenHash, base); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired absolute session error=%v, want unauthorized", err)
	}
}

func TestRegistrationExpiryResendAndDurableQuotas(t *testing.T) {
	r := registrationTestRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	first := verificationHash("first")
	second := verificationHash("second")
	if ok, err := r.PrepareRegistration(ctx, "owner@example.com", first, now, now.Add(30*time.Minute)); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := r.PrepareRegistration(ctx, "owner@example.com", second, now.Add(time.Second), now.Add(30*time.Minute)); err != nil || ok {
		t.Fatal("cooldown not enforced", err)
	}
	later := now.Add(2 * time.Minute)
	if ok, err := r.PrepareRegistration(ctx, "owner@example.com", second, later, later.Add(30*time.Minute)); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if valid, _ := r.RegistrationTokenValid(ctx, first, later); valid {
		t.Fatal("old token survived resend")
	}
	if valid, _ := r.RegistrationTokenValid(ctx, second, later); !valid {
		t.Fatal("new token invalid")
	}
	if err := r.CompleteRegistration(ctx, second, "person", "person", "hash", later.Add(30*time.Minute)); !errors.Is(err, application.ErrRegistrationToken) {
		t.Fatal("expired token accepted")
	}
	for i := 0; i < 3; i++ {
		at := later.Add(time.Duration(i+1) * 2 * time.Minute)
		if ok, err := r.PrepareRegistration(ctx, "owner@example.com", verificationHash(fmt.Sprint(i)), at, at.Add(30*time.Minute)); err != nil || !ok {
			t.Fatal(ok, err)
		}
	}
	if ok, err := r.PrepareRegistration(ctx, "owner@example.com", first, now.Add(12*time.Minute), now.Add(time.Hour)); err != nil || ok {
		t.Fatal("per-email hourly quota not enforced", err)
	}
	if _, err := r.db.Exec(`INSERT INTO registration_mail_attempts(email_hash,attempted_at) SELECT $1,$2 FROM generate_series(1,95)`, verificationHash("other"), now); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.PrepareRegistration(ctx, "another@example.com", first, now.Add(12*time.Minute), now.Add(time.Hour)); err != nil || ok {
		t.Fatal("global hourly quota not enforced", err)
	}
	noCipher := New(r.db)
	if _, err := noCipher.PrepareRegistration(ctx, "plain@example.com", first, now, now.Add(time.Hour)); err == nil {
		t.Fatal("unencrypted email accepted")
	}
}

func TestRegistrationConcurrentMailQuota(t *testing.T) {
	r := registrationTestRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := r.db.Exec(`INSERT INTO registration_mail_attempts(email_hash,attempted_at) SELECT $1,$2 FROM generate_series(1,99)`, verificationHash("other"), now); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Independent repository instances model different backend replicas.
			ok, err := New(r.db, r.secretCipher).PrepareRegistration(ctx, fmt.Sprintf("owner%d@example.com", i), verificationHash(fmt.Sprint(i)), now, now.Add(time.Hour))
			if err != nil {
				t.Error(err)
			}
			results <- ok
		}(i)
	}
	wg.Wait()
	close(results)
	accepted := 0
	for ok := range results {
		if ok {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("concurrent quota admitted %d requests, want 1", accepted)
	}
	if _, err := r.db.Exec(`INSERT INTO registration_mail_attempts(email_hash,attempted_at) SELECT $1,$2 FROM generate_series(1,400)`, verificationHash("other"), now); err != nil {
		t.Fatal(err)
	}
	later := now.Add(2 * time.Hour)
	if ok, err := r.PrepareRegistration(ctx, "new@example.com", verificationHash("daily"), later, later.Add(time.Hour)); err != nil || ok {
		t.Fatal("daily quota not enforced", err)
	}
}
