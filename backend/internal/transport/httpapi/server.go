package httpapi

import (
	"context"
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

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

type Config struct {
	StaticDir            string
	CORSOrigin           string
	FatSecretCallbackURL string
	PublicURL            string
	Production           bool
	TrustProxy           bool
	SecureCookies        bool
	Logs                 LogSource
}

type Server struct {
	registrationGuard     *registrationRateGuard
	registrationMailSlots chan struct{}
	service               *application.Service
	config                Config
	loginLimiter          *loginRateLimiter
	loginSlots            chan struct{}
	attachmentUploadSlots chan struct{}
	publicOrigin          string
	publicHost            string
	logs                  LogSource
}

func New(service *application.Service, config Config) http.Handler {
	server := &Server{
		registrationGuard:     &registrationRateGuard{limiter: newLoginRateLimiter()},
		registrationMailSlots: make(chan struct{}, 2),
		service:               service,
		config:                config,
		loginLimiter:          newLoginRateLimiter(),
		logs:                  config.Logs,
		// Password hashing is deliberately expensive. Bound concurrent hashes so
		// a distributed login flood cannot exhaust all CPU and starve the API.
		loginSlots: make(chan struct{}, 4),
		// A 50 MB upload is intentionally supported. Bound concurrent in-memory
		// copies so a few parallel uploads cannot exhaust a small VPS.
		attachmentUploadSlots: make(chan struct{}, 2),
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
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /api/auth/session", s.authSession)
	mux.HandleFunc("POST /api/auth/login", s.authLogin)
	mux.HandleFunc("POST /api/auth/logout", s.authLogout)
	mux.HandleFunc("GET /api/auth/registration", s.registrationConfig)
	mux.HandleFunc("POST /api/auth/registration/request", s.registrationRequest)
	mux.HandleFunc("POST /api/auth/registration/complete", s.registrationComplete)

	mux.HandleFunc("GET /api/state", s.getState)

	mux.HandleFunc("GET /api/integrations/fatsecret/status", s.fatSecretStatus)
	mux.HandleFunc("POST /api/integrations/fatsecret/connect", s.fatSecretConnect)
	mux.HandleFunc("GET /api/integrations/fatsecret/callback", s.fatSecretCallback)
	mux.HandleFunc("DELETE /api/integrations/fatsecret", s.fatSecretDisconnect)
	mux.HandleFunc("GET /api/integrations/fatsecret/nutrition", s.fatSecretNutrition)
	mux.HandleFunc("GET /api/integrations/fatsecret/foods/search", s.fatSecretFoodSearch)
	mux.HandleFunc("GET /api/integrations/fatsecret/foods/recent", s.fatSecretRecentFoods)
	mux.HandleFunc("GET /api/integrations/fatsecret/foods/barcode/{barcode}", s.fatSecretBarcodeFood)
	mux.HandleFunc("GET /api/integrations/fatsecret/foods/{id}", s.fatSecretFood)
	mux.HandleFunc("POST /api/integrations/fatsecret/entries", s.fatSecretCreateEntry)
	mux.HandleFunc("PUT /api/integrations/fatsecret/entries/{id}", s.fatSecretUpdateEntry)
	mux.HandleFunc("DELETE /api/integrations/fatsecret/entries/{id}", s.fatSecretDeleteEntry)
	mux.HandleFunc("GET /api/foods/search", s.foodSearch)
	mux.HandleFunc("GET /api/foods/recent", s.recentFoods)
	mux.HandleFunc("GET /api/foods/candidates", s.foodCandidates)
	mux.HandleFunc("GET /api/foods/barcode", s.foodBarcode)
	mux.HandleFunc("GET /api/foods/barcode/{barcode}", s.foodBarcode)
	mux.HandleFunc("POST /api/foods/barcode-links", s.linkFoodBarcode)
	mux.HandleFunc("GET /api/foods/{id}", s.foodCatalogItem)
	mux.HandleFunc("DELETE /api/foods/{id}", s.deleteFoodCatalogItem)
	mux.HandleFunc("POST /api/foods", s.createFoodCatalogItem)
	mux.HandleFunc("GET /api/admin/foods", s.adminFoodCatalog)
	mux.HandleFunc("GET /api/admin/logs", s.adminLogs)
	mux.HandleFunc("PUT /api/admin/foods/{id}", s.updateUserFoodReview)
	mux.HandleFunc("DELETE /api/admin/foods/{id}", s.rejectUserFood)
	mux.HandleFunc("POST /api/admin/foods/{id}/promote", s.promoteUserFood)
	mux.HandleFunc("GET /api/nutrition/local", s.localNutrition)
	mux.HandleFunc("POST /api/nutrition/local/entries", s.createLocalNutritionEntry)
	mux.HandleFunc("PUT /api/nutrition/local/entries/{id}", s.updateLocalNutritionEntry)
	mux.HandleFunc("DELETE /api/nutrition/local/entries/{id}", s.deleteLocalNutritionEntry)

	mux.HandleFunc("GET /api/trackers", s.getTrackers)
	mux.HandleFunc("PUT /api/trackers/weight/{date}", s.putWeight)
	mux.HandleFunc("PUT /api/trackers/water/{date}", s.putWater)
	mux.HandleFunc("PUT /api/trackers/calorie-goal", s.putCalorieGoal)
	mux.HandleFunc("POST /api/trackers/custom", s.createCustomTracker)
	mux.HandleFunc("PUT /api/trackers/custom/{id}", s.updateCustomTracker)
	mux.HandleFunc("POST /api/trackers/custom/{id}/step", s.stepCustomTracker)
	mux.HandleFunc("DELETE /api/trackers/custom/{id}", s.deleteCustomTracker)
	mux.HandleFunc("GET /api/trackers/github", s.getGitHubTracker)
	mux.HandleFunc("PUT /api/trackers/github", s.putGitHubTracker)
	mux.HandleFunc("DELETE /api/trackers/github", s.deleteGitHubTracker)
	mux.HandleFunc("GET /api/trackers/reminders", s.getTrackerReminders)
	mux.HandleFunc("PUT /api/trackers/reminders", s.putTrackerReminder)
	mux.HandleFunc("DELETE /api/trackers/reminders", s.deleteTrackerReminder)
	mux.HandleFunc("GET /api/time-tracker", s.getTimeTracker)
	mux.HandleFunc("POST /api/time-tracker/activities", s.createTimeActivity)
	mux.HandleFunc("PUT /api/time-tracker/activities/{id}", s.updateTimeActivity)
	mux.HandleFunc("DELETE /api/time-tracker/activities/{id}", s.deleteTimeActivity)
	mux.HandleFunc("POST /api/time-tracker/activities/{id}/start", s.startTimeActivity)
	mux.HandleFunc("POST /api/time-tracker/pause", s.pauseTimeActivity)
	mux.HandleFunc("POST /api/time-tracker/finish", s.stopTimeActivity)
	mux.HandleFunc("GET /api/time-tracker/statistics", s.getTimeStatistics)
	mux.HandleFunc("GET /api/time-tracker/statistics/activities/{id}", s.getTimeActivityStatistics)

	mux.HandleFunc("GET /api/task-categories", s.getTaskCategories)
	mux.HandleFunc("POST /api/task-categories", s.createTaskCategory)
	mux.HandleFunc("DELETE /api/task-categories/{id}", s.deleteTaskCategory)

	mux.HandleFunc("GET /api/notifications/config", s.notificationConfig)
	mux.HandleFunc("POST /api/notifications/subscriptions", s.savePushSubscription)
	mux.HandleFunc("DELETE /api/notifications/subscriptions", s.deletePushSubscription)

	mux.HandleFunc("GET /api/tasks", s.getTasks)
	mux.HandleFunc("POST /api/tasks", s.createTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.updateTask)
	mux.HandleFunc("PUT /api/tasks/order", s.swapTaskOrder)
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
	mux.HandleFunc("GET /api/work/projects", s.getWorkProjects)
	mux.HandleFunc("POST /api/work/projects", s.createWorkProject)
	mux.HandleFunc("GET /api/work/projects/{id}", s.getWorkProject)
	mux.HandleFunc("GET /api/work/projects/{id}/core", s.getWorkProjectCore)
	mux.HandleFunc("GET /api/work/projects/{id}/revision", s.getWorkProjectRevision)
	mux.HandleFunc("GET /api/work/projects/{id}/resources", s.getWorkProjectResources)
	mux.HandleFunc("GET /api/work/projects/{id}/resource-summary", s.getWorkProjectResourceSummary)
	mux.HandleFunc("PUT /api/work/projects/{id}", s.updateWorkProject)
	mux.HandleFunc("DELETE /api/work/projects/{id}", s.deleteWorkProject)
	mux.HandleFunc("POST /api/work/projects/{id}/sections", s.createWorkSection)
	mux.HandleFunc("PUT /api/work/sections/{id}", s.updateWorkSection)
	mux.HandleFunc("PUT /api/work/sections/{id}/completion-target", s.updateWorkSectionCompletion)
	mux.HandleFunc("DELETE /api/work/sections/{id}", s.deleteWorkSection)
	mux.HandleFunc("POST /api/work/projects/{id}/tasks", s.createWorkTask)
	mux.HandleFunc("PUT /api/work/tasks/{id}", s.updateWorkTask)
	mux.HandleFunc("POST /api/work/tasks/{id}/claim", s.claimWorkTask)
	mux.HandleFunc("DELETE /api/work/tasks/{id}", s.deleteWorkTask)
	mux.HandleFunc("POST /api/work/tasks/{id}/comments", s.createWorkComment)
	mux.HandleFunc("GET /api/work/tasks/{id}/comments", s.getWorkTaskComments)
	mux.HandleFunc("POST /api/work/projects/{id}/invites", s.createWorkInvite)
	mux.HandleFunc("DELETE /api/work/projects/{id}/invites", s.revokeWorkInvites)
	mux.HandleFunc("DELETE /api/work/projects/{id}/members/{userId}", s.removeWorkMember)
	mux.HandleFunc("DELETE /api/work/projects/{id}/membership", s.leaveWorkProject)
	mux.HandleFunc("POST /api/work/invites/{token}/accept", s.acceptWorkInvite)
	mux.HandleFunc("POST /api/work/invites/accept", s.acceptWorkInvite)
	mux.HandleFunc("POST /api/work/projects/{id}/attachments", s.createWorkAttachment)
	mux.HandleFunc("GET /api/work/attachments/{id}", s.downloadWorkAttachment)
	mux.HandleFunc("DELETE /api/work/attachments/{id}", s.deleteWorkAttachment)
	mux.HandleFunc("POST /api/work/projects/{id}/links", s.createWorkLink)
	mux.HandleFunc("DELETE /api/work/links/{id}", s.deleteWorkLink)
	mux.HandleFunc("POST /api/work/projects/{id}/notes", s.createWorkNote)
	mux.HandleFunc("GET /api/work/notes/{id}", s.getWorkNote)
	mux.HandleFunc("PUT /api/work/notes/{id}", s.updateWorkNote)
	mux.HandleFunc("DELETE /api/work/notes/{id}", s.deleteWorkNote)

	mux.HandleFunc("PUT /api/profile", s.updateProfile)
	mux.HandleFunc("PUT /api/profile/work-visibility", s.updateWorkProfile)
	mux.HandleFunc("PUT /api/profile/bottom-navigation", s.updateBottomNavigation)
	mux.HandleFunc("PUT /api/profile/workspace", s.updateWorkspacePreferences)
	mux.HandleFunc("PUT /api/photo", s.updatePhoto)
	mux.HandleFunc("PUT /api/signature", s.updateSignature)
	mux.HandleFunc("POST /api/reset", s.reset)
}

const legacySessionCookieName = "avatar_id_session"
const secureSessionCookieName = "__Host-avatar_id_session"

type authRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.service.Ready(ctx); err != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
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
	s.refreshSessionCookie(w, token)
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
			r.URL.Path == "/api/auth/registration" ||
			r.URL.Path == "/api/auth/registration/request" ||
			r.URL.Path == "/api/auth/registration/complete" ||
			r.URL.Path == "/api/integrations/fatsecret/callback" {
			next.ServeHTTP(w, r)
			return
		}
		token := s.sessionToken(r)
		user, err := s.service.Authenticate(r.Context(), token)
		if err != nil {
			s.clearSessionCookie(w)
			http.Error(w, "требуется вход", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/auth/logout" {
			s.refreshSessionCookie(w, token)
		}
		next.ServeHTTP(w, r.WithContext(application.WithUser(r.Context(), user)))
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
		MaxAge:   int(domain.AuthSessionIdleTimeout / time.Second),
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.config.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) refreshSessionCookie(w http.ResponseWriter, token string) {
	s.setSessionCookie(w, token, time.Now().Add(domain.AuthSessionIdleTimeout).UTC().Format(time.RFC3339))
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
	var value domain.State
	var err error
	if r.URL.Query().Get("includeActiveTasks") == "false" {
		value, err = s.service.StateWithoutActiveTasks(r.Context())
	} else {
		value, err = s.service.State(r.Context())
	}
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

func (s *Server) fatSecretFoodSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	exact := r.URL.Query().Get("exact") == "1"
	page := 0
	if rawPage := strings.TrimSpace(r.URL.Query().Get("page")); rawPage != "" {
		parsedPage, err := strconv.Atoi(rawPage)
		if err != nil {
			http.Error(w, "page must be a non-negative integer", http.StatusBadRequest)
			return
		}
		page = parsedPage
	}
	value, err := s.service.SearchFoods(r.Context(), query, exact, page)
	if err != nil {
		s.writeFatSecretError(w, r, err, "не удалось найти продукты")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) fatSecretFood(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.Food(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writeFatSecretError(w, r, err, "не удалось загрузить продукт")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) fatSecretBarcodeFood(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.BarcodeFood(r.Context(), strings.TrimSpace(r.PathValue("barcode")))
	if err != nil {
		s.writeFatSecretError(w, r, err, "продукт с таким штрихкодом не найден")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) fatSecretRecentFoods(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.RecentFoods(r.Context(), strings.TrimSpace(r.URL.Query().Get("meal")))
	if err != nil {
		s.writeFatSecretError(w, r, err, "не удалось загрузить недавние продукты")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) fatSecretCreateEntry(w http.ResponseWriter, r *http.Request) {
	var input domain.FoodEntryInput
	if err := decodeJSON(w, r, 8_000, &input); err != nil {
		http.Error(w, "bad food entry json", http.StatusBadRequest)
		return
	}
	value, saved, err := s.service.CreateFoodEntry(r.Context(), input)
	if err != nil {
		if saved {
			log.Printf("fatsecret entry saved but diary refresh failed request_id=%s: %v", requestID(r), err)
			w.Header().Set("Cache-Control", "private, no-store")
			writeJSON(w, http.StatusCreated, value)
			return
		}
		s.writeFatSecretError(w, r, err, "не удалось добавить продукт")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) fatSecretUpdateEntry(w http.ResponseWriter, r *http.Request) {
	var input domain.FoodEntryUpdate
	if err := decodeJSON(w, r, 8_000, &input); err != nil {
		http.Error(w, "bad food entry json", http.StatusBadRequest)
		return
	}
	value, err := s.service.UpdateFoodEntry(r.Context(), strings.TrimSpace(r.PathValue("id")), input, strings.TrimSpace(r.URL.Query().Get("date")))
	if err != nil {
		s.writeFatSecretError(w, r, err, "не удалось изменить продукт")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) fatSecretDeleteEntry(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.DeleteFoodEntry(r.Context(), strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.URL.Query().Get("date")))
	if err != nil {
		s.writeFatSecretError(w, r, err, "не удалось удалить продукт")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) writeFatSecretError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrUnauthorized) {
		writeError(w, err)
		return
	}
	log.Printf("fatsecret operation request_id=%s: %v", requestID(r), err)
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "scope") || strings.Contains(lower, "premier") || strings.Contains(lower, "not permitted") || strings.Contains(lower, "invalid region") || strings.Contains(lower, "localization") {
		http.Error(w, "Для российской базы нужен доступ FatSecret к региону RU. Это право выдаётся для Consumer Key отдельно.", http.StatusBadGateway)
		return
	}
	var coded interface{ ProviderErrorCode() string }
	if errors.As(err, &coded) {
		switch coded.ProviderErrorCode() {
		case "5":
			http.Error(w, "FatSecret отклонил Consumer Key (код 5). Проверьте OAuth 1.0 Consumer Key в настройках сервера.", http.StatusBadGateway)
			return
		case "6":
			http.Error(w, "FatSecret отклонил время запроса (код 6). Проверьте синхронизацию времени сервера.", http.StatusBadGateway)
			return
		case "8":
			http.Error(w, "FatSecret отклонил подпись OAuth 1.0 (код 8). Проверьте Consumer Secret и переподключите интеграцию.", http.StatusBadGateway)
			return
		case "9":
			http.Error(w, "Токен подключённого аккаунта FatSecret недействителен (код 9). Отключите и подключите FatSecret заново.", http.StatusBadGateway)
			return
		case "11", "12":
			http.Error(w, "Достигнут лимит запросов FatSecret (код "+coded.ProviderErrorCode()+"). Повторите позже.", http.StatusTooManyRequests)
			return
		case "14", "208":
			http.Error(w, "Для российской базы нужен доступ FatSecret к региону RU (код "+coded.ProviderErrorCode()+"). Это право выдаётся для Consumer Key отдельно.", http.StatusBadGateway)
			return
		}
		if code := strings.TrimSpace(coded.ProviderErrorCode()); code != "" {
			http.Error(w, fallback+". FatSecret вернул код "+code+".", http.StatusBadGateway)
			return
		}
	}
	http.Error(w, fallback, http.StatusBadGateway)
}

func (s *Server) getTrackers(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.Trackers(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (s *Server) putCalorieGoal(w http.ResponseWriter, r *http.Request) {
	var body domain.NutritionGoals
	if err := decodeJSON(w, r, 4_000, &body); err != nil {
		http.Error(w, "bad nutrition goals json", http.StatusBadRequest)
		return
	}
	value, err := s.service.UpdateNutritionGoals(r.Context(), body)
	respond(w, value, err, http.StatusOK)
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

func (s *Server) getGitHubTracker(w http.ResponseWriter, r *http.Request) {
	value, err := s.service.GitHubTracker(r.Context(), r.URL.Query().Get("refresh") == "1")
	s.respondGitHubTracker(w, value, err)
}

func (s *Server) putGitHubTracker(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
	}
	if err := decodeJSON(w, r, 4_000, &body); err != nil {
		http.Error(w, "bad github tracker json", http.StatusBadRequest)
		return
	}
	value, err := s.service.SaveGitHubTracker(r.Context(), body.Username)
	s.respondGitHubTracker(w, value, err)
}

func (s *Server) deleteGitHubTracker(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeleteGitHubTracker(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) respondGitHubTracker(w http.ResponseWriter, value domain.GitHubTrackerState, err error) {
	if err == nil {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, value)
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		http.Error(w, "пользователь GitHub не найден", http.StatusNotFound)
		return
	}
	if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrConflict) {
		writeError(w, err)
		return
	}
	log.Printf("github tracker: %s", sanitizeLogText(err.Error(), 2_000))
	http.Error(w, "не удалось получить публичную активность GitHub", http.StatusBadGateway)
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

func (s *Server) swapTaskOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      int64 `json:"id"`
		OtherID int64 `json:"otherId"`
	}
	if err := decodeJSON(w, r, 4_000, &body); err != nil {
		http.Error(w, "bad task order json", http.StatusBadRequest)
		return
	}
	value, err := s.service.SwapTaskOrder(r.Context(), body.ID, body.OtherID)
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

func (s *Server) updateWorkProfile(w http.ResponseWriter, r *http.Request) {
	var input domain.WorkProfileInput
	if err := decodeJSON(w, r, 6_000_000, &input); err != nil {
		http.Error(w, "bad work profile json or photo too large", http.StatusBadRequest)
		return
	}
	if err := s.service.UpdateWorkProfile(r.Context(), input); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateBottomNavigation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []string `json:"items"`
	}
	if err := decodeJSON(w, r, 2_000, &body); err != nil {
		http.Error(w, "bad bottom navigation json", http.StatusBadRequest)
		return
	}
	if err := s.service.UpdateBottomNavigation(r.Context(), body.Items); err != nil {
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
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
		message = "недостаточно прав"
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
				} else if strings.HasPrefix(relative, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
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
