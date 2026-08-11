package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSafeReturnTo(t *testing.T) {
	valid := map[string]string{
		"/":                     "/",
		"/profile?tab=ticktick": "/profile?tab=ticktick",
	}
	for input, expected := range valid {
		if got := safeReturnTo(input); got != expected {
			t.Fatalf("safeReturnTo(%q)=%q, want %q", input, got, expected)
		}
	}
	for _, input := range []string{
		"https://evil.example/", "//evil.example", `\\evil.example`,
		"/%2f%2fevil.example", "/%5cevil.example", "/%0d%0aInjected",
		"/%2e%2e/admin", "/ok\nInjected: x", "javascript:alert(1)",
	} {
		if got := safeReturnTo(input); got != "/" {
			t.Fatalf("unsafe return URL accepted: %q -> %q", input, got)
		}
	}
}

func TestCSRFMiddleware(t *testing.T) {
	server := &Server{config: Config{PublicURL: "https://identity.example.com"}, publicOrigin: "https://identity.example.com"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := server.csrf(next)

	tests := []struct {
		name   string
		origin string
		marker string
		want   int
	}{
		{"same origin", "https://identity.example.com", "1", http.StatusNoContent},
		{"wrong origin", "https://evil.example", "1", http.StatusForbidden},
		{"missing marker", "https://identity.example.com", "", http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "https://identity.example.com/api/tasks", strings.NewReader("{}"))
			req.Header.Set("Origin", test.origin)
			if test.marker != "" {
				req.Header.Set(requestMarkerHeader, test.marker)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != test.want {
				t.Fatalf("got %d, want %d", response.Code, test.want)
			}
		})
	}
}

func TestProductionCORSNeverUsesWildcardCredentials(t *testing.T) {
	server := &Server{config: Config{CORSOrigin: "*", Production: true}}
	handler := server.cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	req := httptest.NewRequest(http.MethodGet, "https://identity.example.com/api/state", nil)
	req.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if value := response.Header().Get("Access-Control-Allow-Origin"); value != "" {
		t.Fatalf("unexpected production CORS header %q", value)
	}
}

func TestSecurityHeaders(t *testing.T) {
	server := &Server{config: Config{Production: true}}
	handler := server.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://identity.example.com/api/state", nil))
	for _, header := range []string{"Content-Security-Policy", "Strict-Transport-Security", "X-Content-Type-Options", "X-Frame-Options", "Cache-Control"} {
		if response.Header().Get(header) == "" {
			t.Fatalf("missing %s", header)
		}
	}
}

func TestClientIPBehindTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://identity.example.com/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.10")
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 203.0.113.11")
	if got := clientIP(req, true); got != "203.0.113.10" {
		t.Fatalf("trusted proxy IP=%q", got)
	}
	if got := clientIP(req, false); got != "127.0.0.1" {
		t.Fatalf("untrusted proxy headers were used: %q", got)
	}

	req.Header.Del("X-Real-IP")
	if got := clientIP(req, true); got != "203.0.113.11" {
		t.Fatalf("expected right-most forwarded address, got %q", got)
	}
}

func TestLoginLimiterKeyDoesNotContainRawLogin(t *testing.T) {
	combined, ipKey := loginLimiterKeys("203.0.113.10", "  VerySecretLogin  ")
	if strings.Contains(strings.ToLower(combined), "verysecretlogin") {
		t.Fatalf("raw login leaked into limiter key: %q", combined)
	}
	if !strings.HasPrefix(combined, "login:203.0.113.10:") || ipKey != "ip:203.0.113.10" {
		t.Fatalf("unexpected limiter keys: %q %q", combined, ipKey)
	}
}

func TestLoginLimiterIsMemoryBounded(t *testing.T) {
	limiter := newLoginRateLimiter()
	for i := 0; i < 12_000; i++ {
		limiter.failure("ip:shared", "login:key:"+strconv.Itoa(i))
	}
	if len(limiter.entries) > maxLoginLimiterKeys {
		t.Fatalf("limiter map grew to %d entries", len(limiter.entries))
	}
	if _, exists := limiter.entries["login:key:11999"]; !exists {
		t.Fatal("new login attempt was dropped after limiter reached its memory bound")
	}
}

func FuzzSafeReturnTo(f *testing.F) {
	for _, seed := range []string{"/", "/profile?tab=ticktick", "//evil.example", "/%2e%2e/admin", "javascript:alert(1)", "\\\\evil"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		got := safeReturnTo(input)
		if got == "" || !strings.HasPrefix(got, "/") || strings.HasPrefix(got, "//") || strings.ContainsAny(got, "\\\r\n\x00") {
			t.Fatalf("unsafe result %q for %q", got, input)
		}
	})
}

func TestValidRequestID(t *testing.T) {
	for _, value := range []string{"request-123", "ABC_def.2026"} {
		if !validRequestID(value) {
			t.Fatalf("valid request ID rejected: %q", value)
		}
	}
	for _, value := range []string{"short", "request id", "request:123", "request\x1b[31m", strings.Repeat("a", 81)} {
		if validRequestID(value) {
			t.Fatalf("unsafe request ID accepted: %q", value)
		}
	}
}

func TestUserActionGuardBlocksConcurrentAndRapidRequests(t *testing.T) {
	guard := newUserActionGuard(50 * time.Millisecond)
	release, _, ok := guard.begin(7)
	if !ok {
		t.Fatal("first request was rejected")
	}
	if _, _, ok := guard.begin(7); ok {
		t.Fatal("concurrent request was accepted")
	}
	if _, _, ok := guard.begin(8); !ok {
		t.Fatal("different user was incorrectly blocked")
	}
	release()
	if _, retry, ok := guard.begin(7); ok || retry <= 0 {
		t.Fatalf("cooldown was not enforced: ok=%t retry=%s", ok, retry)
	}
	time.Sleep(60 * time.Millisecond)
	releaseAgain, _, ok := guard.begin(7)
	if !ok {
		t.Fatal("request remained blocked after cooldown")
	}
	releaseAgain()
}
