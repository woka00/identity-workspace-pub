package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

type registrationRateGuard struct {
	mu      sync.Mutex
	limiter *loginRateLimiter
}

func (g *registrationRateGuard) take(ip, action string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ipKey := loginLimiterKeys(ip, "")
	keys := []string{action + ipKey, action + ":global"}
	allowed, retry := g.limiter.allow(keys...)
	if allowed {
		g.limiter.failure(keys...)
	}
	return allowed, retry
}

func (s *Server) registrationConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": s.service.RegistrationEnabled()})
}

func (s *Server) registrationAllowed(w http.ResponseWriter, r *http.Request, action string) bool {
	w.Header().Set("Cache-Control", "no-store")
	if !s.service.RegistrationEnabled() {
		http.Error(w, application.ErrRegistrationDisabled.Error(), http.StatusServiceUnavailable)
		return false
	}
	if allowed, retry := s.registrationGuard.take(clientIP(r, s.config.TrustProxy), action); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
		http.Error(w, "слишком много запросов; повторите позже", http.StatusTooManyRequests)
		return false
	}
	return true
}

func (s *Server) registrationRequest(w http.ResponseWriter, r *http.Request) {
	if !s.registrationAllowed(w, r, "email") {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, 2048, &body); err != nil {
		http.Error(w, "неверный формат данных", http.StatusBadRequest)
		return
	}
	select {
	case s.registrationMailSlots <- struct{}{}:
		defer func() { <-s.registrationMailSlots }()
	default:
		w.Header().Set("Retry-After", "10")
		http.Error(w, "сервер занят; повторите позже", http.StatusTooManyRequests)
		return
	}
	if err := s.service.RequestRegistration(r.Context(), body.Email); err != nil {
		if errors.Is(err, domain.ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Do not expose provider responses or recipient/account existence.
		http.Error(w, "не удалось отправить письмо; попробуйте через минуту", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "Если отправка доступна, письмо придёт в ближайшее время. Проверьте входящие и спам. Повторный запрос доступен через минуту."})
}

func (s *Server) registrationComplete(w http.ResponseWriter, r *http.Request) {
	if !s.registrationAllowed(w, r, "complete") {
		return
	}
	var body struct {
		Token    string `json:"token"`
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, 4096, &body); err != nil {
		http.Error(w, "неверный формат данных", http.StatusBadRequest)
		return
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		http.Error(w, "сервер занят; повторите позже", http.StatusTooManyRequests)
		return
	}
	err := s.service.CompleteRegistration(r.Context(), body.Token, body.Login, body.Password)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrRegistrationToken), errors.Is(err, domain.ErrInvalidInput):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, application.ErrRegistrationLogin), errors.Is(err, application.ErrRegistrationEmail):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, "не удалось создать аккаунт; попробуйте позже", http.StatusInternalServerError)
		}
		return
	}
	// Explicit login after creation avoids replacing an existing browser session.
	w.WriteHeader(http.StatusNoContent)
}
