package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"identity-workspace/internal/application"
	"identity-workspace/internal/domain"
)

type Config struct {
	StaticDir            string
	CORSOrigin           string
	FatSecretCallbackURL string
	TickTickCallbackURL  string
	PublicURL            string
	Production           bool
	TrustProxy           bool
	SecureCookies        bool
}

type Server struct {
	service           *application.Service
	config            Config
	loginLimiter      *loginRateLimiter
	loginSlots        chan struct{}
	tickTickSyncGuard *userActionGuard
	publicOrigin      string
	publicHost        string
}

func New(service *application.Service, config Config) http.Handler {
	server := &Server{
		service:           service,
		config:            config,
		loginLimiter:      newLoginRateLimiter(),
		tickTickSyncGuard: newUserActionGuard(15 * time.Second),
		// Password hashing is deliberately expensive. Bound concurrent hashes so
		// a distributed login flood cannot exhaust all CPU and starve the API.
		loginSlots: make(chan struct{}, 4),
	}
	if parsed, err := url.Parse(strings.TrimSpace(config.PublicURL)); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		server.publicOrigin = parsed.Scheme + "://" + parsed.Host
		server.publicHost = parsed.Host
	}
	mux := http.NewServeMux()
	server.routes(mux)
	server.mountStatic(mux)
	handler := server.authMiddleware(mux)
	handler = server.csrf(handler)
	handler = server.cors(handler)
	handler = server.hostGuard(handler)
	handler = server.securityHeaders(handler)
	handler = server.recovery(handler)
	handler = server.requestContext(handler)
	return handler
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/auth/session", s.authSession)
	mux.HandleFunc("POST /api/auth/login", s.authLogin)
	mux.HandleFunc("POST /api/auth/logout", s.authLogout)

	mux.HandleFunc("GET /api/state", s.getState)

	mux.HandleFunc("GET /api/integrations/fatsecret/status", s.fatSecretStatus)
	mux.HandleFunc("POST /api/integrations/fatsecret/connect", s.fatSecretConnect)
	mux.HandleFunc("GET /api/integrations/fatsecret/callback", s.fatSecretCallback)
	mux.HandleFunc("DELETE /api/integrations/fatsecret", s.fatSecretDisconnect)
	mux.HandleFunc("GET /api/integrations/fatsecret/nutrition", s.fatSecretNutrition)

	mux.HandleFunc("GET /api/integrations/ticktick/status", s.tickTickStatus)
	mux.HandleFunc("POST /api/integrations/ticktick/connect", s.tickTickConnect)
	mux.HandleFunc("GET /api/integrations/ticktick/callback", s.tickTickCallback)
	mux.HandleFunc("DELETE /api/integrations/ticktick", s.tickTickDisconnect)
	mux.HandleFunc("POST /api/integrations/ticktick/sync", s.tickTickSync)

	mux.HandleFunc("GET /api/trackers", s.getTrackers)
	mux.HandleFunc("PUT /api/trackers/weight/{date}", s.putWeight)
	mux.HandleFunc("PUT /api/trackers/water/{date}", s.putWater)
	mux.HandleFunc("PUT /api/trackers/calorie-goal", s.putCalorieGoal)
	mux.HandleFunc("POST /api/trackers/custom", s.createCustomTracker)
	mux.HandleFunc("PUT /api/trackers/custom/{id}", s.updateCustomTracker)
	mux.HandleFunc("POST /api/trackers/custom/{id}/step", s.stepCustomTracker)
	mux.HandleFunc("DELETE /api/trackers/custom/{id}", s.deleteCustomTracker)
	mux.HandleFunc("GET /api/trackers/reminders", s.getTrackerReminders)
	mux.HandleFunc("PUT /api/trackers/reminders", s.putTrackerReminder)
	mux.HandleFunc("DELETE /api/trackers/reminders", s.deleteTrackerReminder)

	mux.HandleFunc("GET /api/task-categories", s.getTaskCategories)
	mux.HandleFunc("POST /api/task-categories", s.createTaskCategory)
	mux.HandleFunc("DELETE /api/task-categories/{id}", s.deleteTaskCategory)

	mux.HandleFunc("GET /api/notifications/config", s.notificationConfig)
	mux.HandleFunc("POST /api/notifications/subscriptions", s.savePushSubscription)
	mux.HandleFunc("DELETE /api/notifications/subscriptions", s.deletePushSubscription)

	mux.HandleFunc("GET /api/tasks", s.getTasks)
	mux.HandleFunc("POST /api/tasks", s.createTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.updateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.deleteTask)
	mux.HandleFunc("POST /api/tasks/{id}/complete", s.completeTask)
	mux.HandleFunc("DELETE /api/tasks/{id}/complete", s.uncompleteTask)

	mux.HandleFunc("GET /api/goals", s.getGoals)
	mux.HandleFunc("GET /api/goals/{id}", s.getGoal)
	mux.HandleFunc("POST /api/goals", s.createGoal)
	mux.HandleFunc("PUT /api/goals/order", s.reorderGoals)
	mux.HandleFunc("PUT /api/goals/{id}", s.updateGoal)
	mux.HandleFunc("DELETE /api/goals/{id}", s.deleteGoal)
	mux.HandleFunc("GET /api/portfolio", s.getPortfolio)

	mux.HandleFunc("PUT /api/profile", s.updateProfile)
	mux.HandleFunc("PUT /api/photo", s.updatePhoto)
	mux.HandleFunc("PUT /api/signature", s.updateSignature)
	mux.HandleFunc("POST /api/reset", s.reset)
}

const legacySessionCookieName = "identity_workspace_session"
const secureSessionCookieName = "__Host-identity_workspace_session"

type authRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) authSession(w http.ResponseWriter, r *http.Request) {
	token := s.sessionToken(r)
	user, err := s.service.Authenticate(r.Context(), token)
	if err != nil {
		s.clearSessionCookie(w)
		http.Error(w, "требуется вход", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var body authRequest
	if err := decodeJSON(w, r, 16_000, &body); err != nil {
		http.Error(w, "неверный формат данных", http.StatusBadRequest)
		return
	}
	ip := clientIP(r, s.config.TrustProxy)
	combinedKey, ipKey := loginLimiterKeys(ip, body.Login)
	if allowed, retry := s.loginLimiter.allow(combinedKey, ipKey); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
		http.Error(w, "слишком много попыток входа, повторите позже", http.StatusTooManyRequests)
		return
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		http.Error(w, "сервер занят проверкой входа, повторите позже", http.StatusTooManyRequests)
		return
	}
	session, err := s.service.Login(r.Context(), body.Login, body.Password)
	if errors.Is(err, domain.ErrUnauthorized) {
		s.loginLimiter.failure(combinedKey, ipKey)
		http.Error(w, "неверный логин или пароль", http.StatusUnauthorized)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	s.loginLimiter.success(combinedKey)
	s.setSessionCookie(w, session.Token, session.ExpiresAt)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.service.Logout(r.Context(), s.sessionToken(r)); err != nil {
		writeError(w, err)
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || !strings.HasPrefix(r.URL.Path, "/api/") ||
			r.URL.Path == "/api/auth/login" ||
			r.URL.Path == "/api/auth/session" ||
			r.URL.Path == "/api/integrations/fatsecret/callback" ||
			r.URL.Path == "/api/integrations/ticktick/callback" {
			next.ServeHTTP(w, r)
			return
		}
		user, err := s.service.Authenticate(r.Context(), s.sessionToken(r))
		if err != nil {
			s.clearSessionCookie(w)
			http.Error(w, "требуется вход", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(domain.WithUserID(r.Context(), user.ID)))
	})
}

func (s *Server) sessionCookieName() string {
	if s.config.SecureCookies {
		return secureSessionCookieName
	}
	return legacySessionCookieName
}

func (s *Server) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(s.sessionCookieName())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token, expiresAt string) {
	expires, _ := time.Parse(time.RFC3339, expiresAt)
	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.config.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	names := []string{s.sessionCookieName()}
	if s.config.SecureCookies {
		names = append(names, legacySessionCookieName)
	}
	for _, name := range names {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(1, 0),
			HttpOnly: true,
			Secure:   s.config.SecureCookies,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func (s *Server) getState(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.State(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) fatSecretStatus(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.FatSecretStatus(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) fatSecretConnect(w http.ResponseWriter, r *http.Request) {
	callbackURL := s.callbackURL(r, s.config.FatSecretCallbackURL, "/api/integrations/fatsecret/callback")
	authorizeURL, err := s.service.BeginFatSecretConnection(r.Context(), callbackURL, safeReturnTo(r.URL.Query().Get("return_to")))
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			http.Error(w, "FatSecret не настроен на сервере", http.StatusServiceUnavailable)
			return
		}
		log.Printf("fatsecret connect request_id=%s: %v", requestID(r), err)
		http.Error(w, "не удалось начать подключение FatSecret", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorizeUrl": authorizeURL})
}

func (s *Server) fatSecretCallback(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("oauth_token"))
	verifier := strings.TrimSpace(r.URL.Query().Get("oauth_verifier"))
	if token == "" || verifier == "" {
		http.Redirect(w, r, "/?fatsecret=denied", http.StatusSeeOther)
		return
	}
	returnTo, err := s.service.CompleteFatSecretConnection(r.Context(), token, verifier)
	if errors.Is(err, domain.ErrNotFound) {
		http.Redirect(w, r, "/?fatsecret=expired", http.StatusSeeOther)
		return
	}
	if err != nil {
		log.Printf("fatsecret callback: %v", err)
		http.Redirect(w, r, addReturnStatus(defaultReturnTo(returnTo), "fatsecret", "error"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, addReturnStatus(defaultReturnTo(returnTo), "fatsecret", "connected"), http.StatusSeeOther)
}

func (s *Server) fatSecretDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DisconnectFatSecret(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fatSecretNutrition(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = s.service.Today()
	}
	value, err := s.service.Nutrition(r.Context(), date)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, err)
			return
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("fatsecret nutrition request_id=%s: %v", requestID(r), err)
		http.Error(w, "не удалось получить данные FatSecret", http.StatusBadGateway)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) tickTickStatus(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.TickTickStatus(r.Context())
	if err == nil {
		w.Header().Set("Cache-Control", "no-store")
	}
	respond(w, value, err, http.StatusOK)
}

func (s *Server) tickTickConnect(w http.ResponseWriter, r *http.Request) {
	callbackURL := s.callbackURL(r, s.config.TickTickCallbackURL, "/api/integrations/ticktick/callback")
	authorizeURL, err := s.service.BeginTickTickConnection(r.Context(), callbackURL, safeReturnTo(r.URL.Query().Get("return_to")))
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			http.Error(w, "TickTick не настроен на сервере", http.StatusServiceUnavailable)
			return
		}
		log.Printf("ticktick connect request_id=%s: %v", requestID(r), err)
		http.Error(w, "не удалось начать подключение TickTick", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorizeUrl": authorizeURL})
}

func (s *Server) tickTickCallback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if state == "" || code == "" || strings.TrimSpace(r.URL.Query().Get("error")) != "" {
		http.Redirect(w, r, "/?ticktick=denied", http.StatusSeeOther)
		return
	}
	returnTo, err := s.service.CompleteTickTickConnection(r.Context(), state, code)
	if errors.Is(err, domain.ErrNotFound) {
		http.Redirect(w, r, "/?ticktick=expired", http.StatusSeeOther)
		return
	}
	if err != nil {
		log.Printf("ticktick callback: %v", err)
		http.Redirect(w, r, addReturnStatus(defaultReturnTo(returnTo), "ticktick", "error"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, addReturnStatus(defaultReturnTo(returnTo), "ticktick", "connected"), http.StatusSeeOther)
}

func (s *Server) tickTickDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DisconnectTickTick(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tickTickSync(w http.ResponseWriter, r *http.Request) {
	userID, err := domain.UserID(r.Context())
	if err != nil {
		http.Error(w, "требуется вход", http.StatusUnauthorized)
		return
	}
	release, retry, allowed := s.tickTickSyncGuard.begin(userID)
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Round(time.Second).Seconds()))))
		http.Error(w, "синхронизация уже выполняется или была запущена недавно", http.StatusTooManyRequests)
		return
	}
	defer release()
	value, err := s.service.SyncTickTick(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) getTrackers(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.Trackers(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) putCalorieGoal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CalorieGoal int `json:"calorieGoal"`
	}
	if err := decodeJSON(w, r, 4_000, &body); err != nil {
		http.Error(w, "bad calorie goal json", http.StatusBadRequest)
		return
	}
	value, err := s.service.UpdateCalorieGoal(r.Context(), body.CalorieGoal)
	respond(w, map[string]int{"calorieGoal": value}, err, http.StatusOK)
}

func (s *Server) putWeight(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WeightKg float64 `json:"weightKg"`
	}
	if err := decodeJSON(w, r, 4_000, &body); err != nil {
		http.Error(w, "bad tracker weight json", http.StatusBadRequest)
		return
	}
	value, err := s.service.UpsertWeight(r.Context(), r.PathValue("date"), body.WeightKg)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) putWater(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Glasses     int `json:"glasses"`
		GoalGlasses int `json:"goalGlasses"`
	}
	if err := decodeJSON(w, r, 4_000, &body); err != nil {
		http.Error(w, "bad tracker water json", http.StatusBadRequest)
		return
	}
	value, err := s.service.UpsertWater(r.Context(), r.PathValue("date"), body.Glasses, body.GoalGlasses)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) getTaskCategories(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.TaskCategories(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) createTaskCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, 4_000, &body); err != nil {
		http.Error(w, "bad category json", http.StatusBadRequest)
		return
	}
	value, err := s.service.CreateTaskCategory(r.Context(), body.Name)
	respond(w, value, err, http.StatusCreated)
}

func (s *Server) deleteTaskCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "task category")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.service.DeleteTaskCategory(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createCustomTracker(w http.ResponseWriter, r *http.Request) {
	var input domain.CustomTrackerInput
	if err := decodeJSON(w, r, 8_000, &input); err != nil {
		http.Error(w, "bad custom tracker json", http.StatusBadRequest)
		return
	}
	value, err := s.service.CreateCustomTracker(r.Context(), input)
	respond(w, value, err, http.StatusCreated)
}

func (s *Server) updateCustomTracker(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "custom tracker")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var input domain.CustomTrackerInput
	if err := decodeJSON(w, r, 8_000, &input); err != nil {
		http.Error(w, "bad custom tracker json", http.StatusBadRequest)
		return
	}
	value, err := s.service.UpdateCustomTracker(r.Context(), id, input)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) stepCustomTracker(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "custom tracker")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var body struct {
		Direction int    `json:"direction"`
		Date      string `json:"date"`
	}
	if err := decodeJSON(w, r, 2_000, &body); err != nil {
		http.Error(w, "bad custom tracker step json", http.StatusBadRequest)
		return
	}
	value, err := s.service.StepCustomTracker(r.Context(), id, body.Date, body.Direction)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) deleteCustomTracker(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "custom tracker")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.service.DeleteCustomTracker(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getTrackerReminders(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.TrackerReminders(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) putTrackerReminder(w http.ResponseWriter, r *http.Request) {
	var input domain.TrackerReminderInput
	if err := decodeJSON(w, r, 4_000, &input); err != nil {
		http.Error(w, "bad tracker reminder json", http.StatusBadRequest)
		return
	}
	value, err := s.service.SaveTrackerReminder(r.Context(), input)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) deleteTrackerReminder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TrackerKey string `json:"trackerKey"`
	}
	if err := decodeJSON(w, r, 2_000, &body); err != nil {
		http.Error(w, "bad tracker reminder json", http.StatusBadRequest)
		return
	}
	if err := s.service.DeleteTrackerReminder(r.Context(), body.TrackerKey); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) notificationConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.service.NotificationConfig())
}

func (s *Server) savePushSubscription(w http.ResponseWriter, r *http.Request) {
	var input domain.PushSubscriptionInput
	if err := decodeJSON(w, r, 8_000, &input); err != nil {
		http.Error(w, "bad push subscription json", http.StatusBadRequest)
		return
	}
	if err := s.service.SavePushSubscription(r.Context(), input, r.UserAgent()); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deletePushSubscription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decodeJSON(w, r, 4_000, &body); err != nil {
		http.Error(w, "bad push subscription json", http.StatusBadRequest)
		return
	}
	if err := s.service.DeletePushSubscription(r.Context(), body.Endpoint); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getTasks(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.Tasks(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var input domain.TaskInput
	if err := decodeJSON(w, r, 16_000, &input); err != nil {
		http.Error(w, "bad task json", http.StatusBadRequest)
		return
	}
	value, err := s.service.CreateTask(r.Context(), input)
	respond(w, value, err, http.StatusCreated)
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "task")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var input domain.TaskInput
	if err := decodeJSON(w, r, 16_000, &input); err != nil {
		http.Error(w, "bad task json", http.StatusBadRequest)
		return
	}
	value, err := s.service.UpdateTask(r.Context(), id, input)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "task")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.service.DeleteTask(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) completeTask(w http.ResponseWriter, r *http.Request) {
	s.setTaskCompleted(w, r, true)
}

func (s *Server) uncompleteTask(w http.ResponseWriter, r *http.Request) {
	s.setTaskCompleted(w, r, false)
}

func (s *Server) setTaskCompleted(w http.ResponseWriter, r *http.Request, completed bool) {
	id, err := pathID(r, "task")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := s.service.SetTaskCompleted(r.Context(), id, completed)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) getGoals(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.Goals(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) getGoal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	value, err := s.service.Goal(r.Context(), id)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) createGoal(w http.ResponseWriter, r *http.Request) {
	var input domain.GoalInput
	if err := decodeJSON(w, r, 64_000, &input); err != nil {
		http.Error(w, "bad project json", http.StatusBadRequest)
		return
	}
	value, err := s.service.CreateGoal(r.Context(), input)
	respond(w, value, err, http.StatusCreated)
}

func (s *Server) reorderGoals(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(w, r, 64_000, &body); err != nil {
		http.Error(w, "bad project order json", http.StatusBadRequest)
		return
	}
	value, err := s.service.ReorderGoals(r.Context(), body.IDs)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) updateGoal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var input domain.GoalInput
	if err := decodeJSON(w, r, 64_000, &input); err != nil {
		http.Error(w, "bad project json", http.StatusBadRequest)
		return
	}
	value, err := s.service.UpdateGoal(r.Context(), id, input)
	respond(w, value, err, http.StatusOK)
}

func (s *Server) deleteGoal(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "project")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.service.DeleteGoal(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getPortfolio(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.Portfolio(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	var profile domain.Profile
	if err := decodeJSON(w, r, 16_000, &profile); err != nil {
		http.Error(w, "bad profile json", http.StatusBadRequest)
		return
	}
	if err := s.service.UpdateProfile(r.Context(), profile); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updatePhoto(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Data string `json:"data"`
	}
	if err := decodeJSON(w, r, 6_000_000, &body); err != nil {
		http.Error(w, "bad json or photo too large (max ~4MB)", http.StatusBadRequest)
		return
	}
	if err := s.service.UpdatePhoto(r.Context(), body.Data); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateSignature(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Data string `json:"data"`
	}
	if err := decodeJSON(w, r, 750_000, &body); err != nil {
		http.Error(w, "bad json or signature too large", http.StatusBadRequest)
		return
	}
	if err := s.service.UpdateSignature(r.Context(), body.Data); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := decodeJSON(w, r, 1_000, &body); err != nil || body.Confirm != "RESET" {
		http.Error(w, "для сброса требуется явное подтверждение", http.StatusBadRequest)
		return
	}
	if err := s.service.Reset(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, limit int64, target any) error {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func pathID(r *http.Request, entity string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s id", entity)
	}
	return id, nil
}

func respond(w http.ResponseWriter, value any, err error, status int) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, status, value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	log.Printf("application error: %s", sanitizeLogText(err.Error(), 2_000))
	status := http.StatusInternalServerError
	message := "внутренняя ошибка сервера"
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		status = http.StatusUnauthorized
		message = "требуется вход"
	case errors.Is(err, domain.ErrInvalidInput):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		message = "объект не найден"
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	}
	http.Error(w, message, status)
}

func sanitizeLogText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, strings.TrimSpace(value))
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}

func requestBaseURL(r *http.Request, trustProxy bool) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if trustProxy {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
			scheme = forwarded
		}
	}
	return scheme + "://" + r.Host
}

func (s *Server) callbackURL(r *http.Request, configured, callbackPath string) string {
	if value := strings.TrimSpace(configured); value != "" {
		return value
	}
	if s.publicOrigin != "" {
		return s.publicOrigin + callbackPath
	}
	return requestBaseURL(r, s.config.TrustProxy) + callbackPath
}

func safeReturnTo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\\\r\n\x00") {
		return "/"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil ||
		!strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") ||
		strings.ContainsAny(parsed.Path, "\\\r\n\x00") {
		return "/"
	}
	// Reject encoded network-path references and ambiguous dot-segment paths.
	// Browsers may normalize encoded backslashes/slashes differently from Go.
	cleaned := path.Clean(parsed.Path)
	if cleaned != parsed.Path && !(parsed.Path != "/" && cleaned+"/" == parsed.Path) {
		return "/"
	}
	return parsed.String()
}

func defaultReturnTo(value string) string {
	return safeReturnTo(value)
}

func addReturnStatus(returnTo, key, status string) string {
	parsed, err := url.Parse(safeReturnTo(returnTo))
	if err != nil {
		return "/"
	}
	query := parsed.Query()
	query.Set(key, status)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (s *Server) mountStatic(mux *http.ServeMux) {
	dir := strings.TrimSpace(s.config.StaticDir)
	if dir == "" {
		return
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return
	}
	indexPath := filepath.Join(absolute, "index.html")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		cleanPath := path.Clean("/" + r.URL.Path)
		relative := strings.TrimPrefix(cleanPath, "/")
		candidate := filepath.Join(absolute, filepath.FromSlash(relative))
		rel, relErr := filepath.Rel(absolute, candidate)
		if relErr == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." {
			if fileInfo, statErr := os.Stat(candidate); statErr == nil && !fileInfo.IsDir() {
				if relative == "manifest.webmanifest" || relative == "sw.js" {
					w.Header().Set("Cache-Control", "no-cache")
				} else {
					w.Header().Set("Cache-Control", "public, max-age=3600")
				}
				http.ServeFile(w, r, candidate)
				return
			}
		}
		if _, statErr := os.Stat(indexPath); statErr != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, indexPath)
	})
}
