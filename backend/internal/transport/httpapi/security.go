package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	requestMarkerHeader = "X-Identity-Workspace-Request"
	maxLoginLimiterKeys = 10_000
)

type loginAttempt struct {
	failures []time.Time
}

type loginRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginAttempt
}

type userActionGuard struct {
	mu          sync.Mutex
	inFlight    map[int64]bool
	nextAllowed map[int64]time.Time
	cooldown    time.Duration
}

func newUserActionGuard(cooldown time.Duration) *userActionGuard {
	return &userActionGuard{
		inFlight:    make(map[int64]bool),
		nextAllowed: make(map[int64]time.Time),
		cooldown:    cooldown,
	}
}

// begin prevents an authenticated user from queueing concurrent expensive
// provider synchronizations and enforces a short server-side cooldown. The
// frontend polls every 30 seconds, so a 15-second cooldown does not affect the
// normal flow but bounds abuse by a stolen session.
func (g *userActionGuard) begin(userID int64) (func(), time.Duration, bool) {
	if userID <= 0 {
		return func() {}, 0, false
	}
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight[userID] {
		return func() {}, 2 * time.Second, false
	}
	if next := g.nextAllowed[userID]; now.Before(next) {
		return func() {}, next.Sub(now), false
	}
	g.inFlight[userID] = true
	released := false
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if released {
			return
		}
		released = true
		delete(g.inFlight, userID)
		g.nextAllowed[userID] = time.Now().Add(g.cooldown)
	}, 0, true
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{entries: make(map[string]*loginAttempt)}
}

func (l *loginRateLimiter) allow(keys ...string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	windowStart := now.Add(-15 * time.Minute)
	limits := []int{8, 30}
	var longest time.Duration
	for index, key := range keys {
		entry := l.entries[key]
		if entry == nil {
			continue
		}
		entry.failures = pruneTimes(entry.failures, windowStart)
		limit := limits[min(index, len(limits)-1)]
		if len(entry.failures) >= limit {
			retry := entry.failures[0].Add(15 * time.Minute).Sub(now)
			if retry > longest {
				longest = retry
			}
		}
	}
	if longest > 0 {
		return false, longest
	}
	return true, 0
}

func (l *loginRateLimiter) failure(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	windowStart := now.Add(-15 * time.Minute)
	if len(l.entries) >= maxLoginLimiterKeys {
		l.pruneAndBound(windowStart)
	}
	for _, key := range keys {
		entry := l.entries[key]
		if entry == nil {
			if len(l.entries) >= maxLoginLimiterKeys {
				l.evictOldest()
			}
			entry = &loginAttempt{}
			l.entries[key] = entry
		}
		entry.failures = append(pruneTimes(entry.failures, windowStart), now)
	}
}

func (l *loginRateLimiter) pruneAndBound(windowStart time.Time) {
	for key, entry := range l.entries {
		entry.failures = pruneTimes(entry.failures, windowStart)
		if len(entry.failures) == 0 {
			delete(l.entries, key)
		}
	}
	for len(l.entries) >= maxLoginLimiterKeys {
		l.evictOldest()
	}
}

func (l *loginRateLimiter) evictOldest() {
	var oldestKey string
	var oldest time.Time
	for key, entry := range l.entries {
		when := time.Time{}
		if len(entry.failures) > 0 {
			when = entry.failures[len(entry.failures)-1]
		}
		if oldestKey == "" || when.Before(oldest) {
			oldestKey = key
			oldest = when
		}
	}
	if oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}

func (l *loginRateLimiter) success(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func pruneTimes(values []time.Time, after time.Time) []time.Time {
	index := 0
	for index < len(values) && values[index].Before(after) {
		index++
	}
	return values[index:]
}

func (s *Server) recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic request_id=%s method=%s path=%s: %v", requestID(r), r.Method, r.URL.Path, recovered)
				http.Error(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !validRequestID(id) {
			buffer := make([]byte, 12)
			if _, err := rand.Read(buffer); err == nil {
				id = hex.EncodeToString(buffer)
			} else {
				id = strconv.FormatInt(time.Now().UnixNano(), 36)
			}
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(withRequestID(r.Context(), id)))
	})
}

func validRequestID(value string) bool {
	if len(value) < 8 || len(value) > 80 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"form-action 'self' https://authentication.fatsecret.com https://ticktick.com",
		"img-src 'self' data: blob:",
		"style-src 'self' 'unsafe-inline'",
		"script-src 'self' 'wasm-unsafe-eval'",
		"connect-src 'self'",
		"worker-src 'self' blob:",
		"child-src 'self' blob:",
		"font-src 'self' data:",
		"manifest-src 'self'",
		"media-src 'none'",
	}, "; ")
	if s.config.Production {
		csp += "; upgrade-insecure-requests"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), browsing-topics=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		if s.config.Production {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostGuard(next http.Handler) http.Handler {
	if !s.config.Production || s.publicHost == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && !strings.EqualFold(strings.TrimSpace(r.Host), s.publicHost) {
			http.Error(w, "неверный host", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions ||
			!strings.HasPrefix(r.URL.Path, "/api/") ||
			r.URL.Path == "/api/integrations/fatsecret/callback" ||
			r.URL.Path == "/api/integrations/ticktick/callback" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get(requestMarkerHeader) != "1" {
			http.Error(w, "запрос отклонён", http.StatusForbidden)
			return
		}
		if fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); fetchSite != "" && fetchSite != "same-origin" && fetchSite != "same-site" && fetchSite != "none" {
			http.Error(w, "межсайтовый запрос отклонён", http.StatusForbidden)
			return
		}
		if !s.sameOriginRequest(r) {
			http.Error(w, "запрос с другого источника отклонён", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sameOriginRequest(r *http.Request) bool {
	expected := s.publicOrigin
	if expected == "" {
		expected = requestBaseURL(r, s.config.TrustProxy)
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return equalOrigin(origin, expected)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		parsed, err := url.Parse(referer)
		if err != nil {
			return false
		}
		return equalOrigin(parsed.Scheme+"://"+parsed.Host, expected)
	}
	// Non-browser clients may omit both headers; the custom request marker is still required.
	return true
}

func equalOrigin(left, right string) bool {
	l, err := url.Parse(left)
	if err != nil || l.Scheme == "" || l.Host == "" || l.User != nil {
		return false
	}
	r, err := url.Parse(right)
	if err != nil || r.Scheme == "" || r.Host == "" || r.User != nil {
		return false
	}
	return strings.EqualFold(l.Scheme, r.Scheme) && strings.EqualFold(l.Host, r.Host)
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := parseOriginList(s.config.CORSOrigin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		matched := ""
		if origin != "" {
			for _, candidate := range allowed {
				if candidate == "*" && !s.config.Production {
					matched = "*"
					break
				}
				if equalOrigin(origin, candidate) {
					matched = origin
					break
				}
			}
		}
		if matched != "" {
			w.Header().Set("Access-Control-Allow-Origin", matched)
			w.Header().Add("Vary", "Origin")
			if matched != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+requestMarkerHeader)
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			if origin != "" && matched == "" {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseOriginList(value string) []string {
	var out []string
	for _, raw := range strings.Split(value, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		out = append(out, strings.TrimRight(raw, "/"))
	}
	return out
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		// Only enable TrustProxy when the application port is reachable solely by
		// a trusted reverse proxy. Prefer the proxy-overwritten X-Real-IP value.
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(realIP) != nil {
			return realIP
		}
		// If a proxy chain is used, the nearest trusted proxy appends the client
		// address at the right. Reading the final syntactically valid value avoids
		// trusting an attacker-supplied left-most entry.
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for index := len(parts) - 1; index >= 0; index-- {
			forwarded := strings.TrimSpace(parts[index])
			if net.ParseIP(forwarded) != nil {
				return forwarded
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func loginLimiterKeys(ip, login string) (string, string) {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(login))))
	return "login:" + ip + ":" + hex.EncodeToString(sum[:12]), "ip:" + ip
}
