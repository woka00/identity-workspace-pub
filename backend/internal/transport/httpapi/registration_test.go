package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"avatar-id/internal/application"
)

type registrationRepoStub struct{ calls atomic.Int32 }

func (s *registrationRepoStub) PrepareRegistration(context.Context, string, string, time.Time, time.Time) (bool, error) {
	s.calls.Add(1)
	return false, nil
}
func (*registrationRepoStub) RegistrationTokenValid(context.Context, string, time.Time) (bool, error) {
	return false, nil
}
func (*registrationRepoStub) CompleteRegistration(context.Context, string, string, string, string, time.Time) error {
	return application.ErrRegistrationToken
}

type registrationMailStub struct{}

func (*registrationMailStub) SendVerification(context.Context, string, string) error { return nil }

func TestRegistrationHTTPGuards(t *testing.T) {
	repo := &registrationRepoStub{}
	registration, _ := application.NewRegistration(repo, &registrationMailStub{}, "https://workspace.example")
	service := application.New(nil, nil, nil).WithRegistration(registration)
	handler := New(service, Config{PublicURL: "https://workspace.example", SecureCookies: true})
	for _, tc := range []struct {
		name, path, origin, marker, body string
		status                           int
	}{
		{"csrf marker", "/api/auth/registration/request", "https://workspace.example", "", `{"email":"user@example.com"}`, 403},
		{"cross origin", "/api/auth/registration/request", "https://evil.example", "1", `{"email":"user@example.com"}`, 403},
		{"unknown property", "/api/auth/registration/request", "https://workspace.example", "1", `{"email":"user@example.com","admin":true}`, 400},
		{"valid request", "/api/auth/registration/request", "https://workspace.example", "1", `{"email":"user@example.com"}`, 202},
		{"invalid token", "/api/auth/registration/complete", "https://workspace.example", "1", `{"token":"invalid","login":"person","password":"long enough password"}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "https://workspace.example"+tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", tc.origin)
			req.Header.Set(requestMarkerHeader, tc.marker)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("registration issued a session")
			}
		})
	}
	if repo.calls.Load() != 1 {
		t.Fatal("unauthorized request reached storage")
	}
	disabled := New(application.New(nil, nil, nil), Config{})
	w := httptest.NewRecorder()
	disabled.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://example/api/auth/registration", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatal("configuration should be disabled")
	}
}

func TestRegistrationLimiterConcurrentRequests(t *testing.T) {
	g := &registrationRateGuard{limiter: newLoginRateLimiter()}
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := g.take("192.0.2.1", "email"); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 8 {
		t.Fatalf("concurrent requests allowed=%d, want 8", allowed.Load())
	}
}
