package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"identity-workspace/internal/application"
	"identity-workspace/internal/domain"
	fatsecretinfra "identity-workspace/internal/infrastructure/fatsecret"
	"identity-workspace/internal/infrastructure/postgres"
	ticktickinfra "identity-workspace/internal/infrastructure/ticktick"
	webpushinfra "identity-workspace/internal/infrastructure/webpush"
	"identity-workspace/internal/transport/httpapi"

	_ "github.com/lib/pq"
)

const defaultDevelopmentDSN = "postgres://identity:identity-local-only@localhost:5432/identity_workspace?sslmode=disable"

type runtimeConfig struct {
	production           bool
	addr                 string
	databaseURL          string
	publicURL            string
	corsOrigin           string
	staticDir            string
	playerTZ             string
	trustProxy           bool
	runMigrations        bool
	dataEncryptionKey    string
	fatSecretKey         string
	fatSecretSecret      string
	fatSecretCallbackURL string
	tickTickClientID     string
	tickTickClientSecret string
	tickTickCallbackURL  string
	vapidPrivateKey      string
	vapidSubject         string
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	db, err := openDatabase(cfg.databaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	cipher, err := postgres.NewSecretCipher(cfg.dataEncryptionKey)
	if err != nil {
		log.Fatalf("DATA_ENCRYPTION_KEY: %v", err)
	}
	repository := postgres.New(db, cipher)

	command := "serve"
	if len(os.Args) > 1 {
		command = strings.ToLower(strings.TrimSpace(os.Args[1]))
	}
	if cfg.runMigrations || command != "serve" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err := repository.Migrate(ctx)
		cancel()
		if err != nil {
			log.Fatalf("migrations: %v", err)
		}
	}

	if command != "serve" {
		if err := runAdminCommand(repository, command, os.Args[2:]); err != nil {
			log.Fatalf("%s: %v", command, err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := repository.ReencryptLegacySecrets(ctx); err != nil {
		cancel()
		log.Fatalf("encrypt existing OAuth credentials: %v", err)
	}
	cancel()

	if cfg.production {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		unrotated, err := repository.UnrotatedEnabledUsers(ctx)
		cancel()
		if err != nil {
			log.Fatalf("check production accounts: %v", err)
		}
		if len(unrotated) > 0 {
			log.Fatalf("production startup blocked: replace development passwords or disable these accounts first: %s", strings.Join(unrotated, ", "))
		}
	}

	loc, err := time.LoadLocation(cfg.playerTZ)
	if err != nil {
		log.Fatalf("bad PLAYER_TZ: %v", err)
	}
	now := func() time.Time { return time.Now().In(loc) }

	fatSecretClient := &fatsecretinfra.Client{
		ConsumerKey:    cfg.fatSecretKey,
		ConsumerSecret: cfg.fatSecretSecret,
	}
	tickTickClient := &ticktickinfra.Client{
		ClientID:     cfg.tickTickClientID,
		ClientSecret: cfg.tickTickClientSecret,
		TimeZone:     cfg.playerTZ,
	}
	pushClient, err := webpushinfra.New(cfg.vapidPrivateKey, cfg.dataEncryptionKey, cfg.vapidSubject)
	if err != nil {
		log.Fatalf("Web Push configuration: %v", err)
	}
	service := application.New(repository, fatSecretClient, now).WithTickTick(tickTickClient).WithPush(pushClient)
	handler := httpapi.New(service, httpapi.Config{
		StaticDir:            cfg.staticDir,
		CORSOrigin:           cfg.corsOrigin,
		FatSecretCallbackURL: cfg.fatSecretCallbackURL,
		TickTickCallbackURL:  cfg.tickTickCallbackURL,
		PublicURL:            cfg.publicURL,
		Production:           cfg.production,
		TrustProxy:           cfg.trustProxy,
		SecureCookies:        cfg.production || strings.HasPrefix(cfg.publicURL, "https://"),
	})

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       75 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go service.RunReminderWorker(workerCtx)

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("identity workspace listening on %s (environment=%s, calendar timezone=%s)", cfg.addr, environmentName(cfg.production), loc)
		serverErrors <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		log.Printf("received %s, shutting down", sig)
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server: %v", err)
		}
		return
	}

	stopWorkers()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		log.Fatalf("graceful shutdown: %v", err)
	}
}

func loadConfig() (runtimeConfig, error) {
	appEnvironment := strings.ToLower(strings.TrimSpace(env("APP_ENV", "development")))
	if appEnvironment != "development" && appEnvironment != "production" {
		return runtimeConfig{}, fmt.Errorf("APP_ENV must be development or production, got %q", appEnvironment)
	}
	production := appEnvironment == "production"
	publicURL, err := validatePublicURL(strings.TrimSpace(env("PUBLIC_URL", "")), production)
	if err != nil {
		return runtimeConfig{}, err
	}

	dsn, err := secretEnv("DATABASE_URL")
	if err != nil {
		return runtimeConfig{}, err
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		dsn = defaultDevelopmentDSN
	}
	if production && usesDevelopmentDatabaseCredentials(dsn) {
		return runtimeConfig{}, errors.New("DATABASE_URL still uses the documented local development credentials")
	}

	dataEncryptionKey, err := secretEnv("DATA_ENCRYPTION_KEY")
	if err != nil {
		return runtimeConfig{}, err
	}
	if production && dataEncryptionKey == "" {
		return runtimeConfig{}, errors.New("DATA_ENCRYPTION_KEY is required in production")
	}

	fatKey, err := secretEnv("FATSECRET_CONSUMER_KEY")
	if err != nil {
		return runtimeConfig{}, err
	}
	fatSecret, err := secretEnv("FATSECRET_CONSUMER_SECRET")
	if err != nil {
		return runtimeConfig{}, err
	}
	if (fatKey == "") != (fatSecret == "") {
		return runtimeConfig{}, errors.New("FATSECRET_CONSUMER_KEY and FATSECRET_CONSUMER_SECRET must be configured together")
	}

	tickID, err := secretEnv("TICKTICK_CLIENT_ID")
	if err != nil {
		return runtimeConfig{}, err
	}
	tickSecret, err := secretEnv("TICKTICK_CLIENT_SECRET")
	if err != nil {
		return runtimeConfig{}, err
	}
	if (tickID == "") != (tickSecret == "") {
		return runtimeConfig{}, errors.New("TICKTICK_CLIENT_ID and TICKTICK_CLIENT_SECRET must be configured together")
	}

	vapidPrivateKey, err := secretEnv("VAPID_PRIVATE_KEY")
	if err != nil {
		return runtimeConfig{}, err
	}
	vapidSubject := strings.TrimSpace(env("VAPID_SUBJECT", ""))
	if vapidSubject == "" {
		if publicURL != "" {
			vapidSubject = publicURL
		} else {
			vapidSubject = "mailto:admin@localhost"
		}
	}

	fatCallback := strings.TrimSpace(env("FATSECRET_CALLBACK_URL", ""))
	tickCallback := strings.TrimSpace(env("TICKTICK_CALLBACK_URL", ""))
	if production {
		if fatKey != "" && fatCallback == "" {
			fatCallback = publicURL + "/api/integrations/fatsecret/callback"
		}
		if tickID != "" && tickCallback == "" {
			tickCallback = publicURL + "/api/integrations/ticktick/callback"
		}
	}
	if err := validateCallbackURL(fatCallback, publicURL, "/api/integrations/fatsecret/callback", production); err != nil {
		return runtimeConfig{}, fmt.Errorf("FATSECRET_CALLBACK_URL: %w", err)
	}
	if err := validateCallbackURL(tickCallback, publicURL, "/api/integrations/ticktick/callback", production); err != nil {
		return runtimeConfig{}, fmt.Errorf("TICKTICK_CALLBACK_URL: %w", err)
	}

	corsOrigin := strings.TrimSpace(env("CORS_ORIGIN", ""))
	if production {
		corsOrigin, err = validateProductionCORS(corsOrigin, publicURL)
		if err != nil {
			return runtimeConfig{}, err
		}
	}
	// Proxy headers are untrusted by default. The production Compose overlay
	// enables them only because port 8080 is bound to loopback behind Nginx.
	trustProxy, err := envBool("TRUST_PROXY", false)
	if err != nil {
		return runtimeConfig{}, err
	}
	// Development may migrate automatically. Production requires an explicit
	// migration command unless the operator deliberately opts in.
	runMigrations, err := envBool("RUN_MIGRATIONS", !production)
	if err != nil {
		return runtimeConfig{}, err
	}

	return runtimeConfig{
		production:           production,
		addr:                 env("ADDR", ":8080"),
		databaseURL:          dsn,
		publicURL:            publicURL,
		corsOrigin:           corsOrigin,
		staticDir:            env("STATIC_DIR", "../frontend/dist"),
		playerTZ:             env("PLAYER_TZ", "Local"),
		trustProxy:           trustProxy,
		runMigrations:        runMigrations,
		dataEncryptionKey:    dataEncryptionKey,
		fatSecretKey:         strings.TrimSpace(fatKey),
		fatSecretSecret:      strings.TrimSpace(fatSecret),
		fatSecretCallbackURL: fatCallback,
		tickTickClientID:     strings.TrimSpace(tickID),
		tickTickClientSecret: strings.TrimSpace(tickSecret),
		tickTickCallbackURL:  tickCallback,
		vapidPrivateKey:      strings.TrimSpace(vapidPrivateKey),
		vapidSubject:         vapidSubject,
	}, nil
}

func openDatabase(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("PostgreSQL unreachable: %w", err)
	}
	return db, nil
}

func runAdminCommand(repository *postgres.Repository, command string, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	switch command {
	case "migrate":
		log.Print("migrations applied successfully")
		return nil
	case "set-password":
		if len(args) != 1 {
			return errors.New("usage: identity-workspace-server set-password <login>; provide IDENTITY_NEW_PASSWORD or IDENTITY_NEW_PASSWORD_FILE")
		}
		password, err := secretEnv("IDENTITY_NEW_PASSWORD")
		if err != nil {
			return err
		}
		if password == "" {
			return errors.New("IDENTITY_NEW_PASSWORD or IDENTITY_NEW_PASSWORD_FILE is required")
		}
		normalized, err := application.NormalizeLoginForAdmin(args[0])
		if err != nil {
			return err
		}
		hash, err := application.HashPasswordForAdmin(password)
		if err != nil {
			return err
		}
		if err := repository.AdminSetPassword(ctx, normalized, hash); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("user %q does not exist", args[0])
			}
			return err
		}
		log.Printf("password replaced and all sessions revoked for %s", args[0])
		return nil
	case "disable-user", "enable-user":
		if len(args) != 1 {
			return fmt.Errorf("usage: identity-workspace-server %s <login>", command)
		}
		normalized, err := application.NormalizeLoginForAdmin(args[0])
		if err != nil {
			return err
		}
		enabled := command == "enable-user"
		if err := repository.AdminSetUserEnabled(ctx, normalized, enabled); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("user %q does not exist", args[0])
			}
			return err
		}
		log.Printf("user %s enabled=%t", args[0], enabled)
		return nil
	case "security-status":
		users, err := repository.UnrotatedEnabledUsers(ctx)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			log.Print("all enabled users have production passwords")
			return nil
		}
		return fmt.Errorf("enabled users still using unconfirmed preview passwords: %s", strings.Join(users, ", "))
	default:
		return errors.New("unknown command; supported: serve, migrate, set-password, enable-user, disable-user, security-status")
	}
}

func validateProductionCORS(raw, publicURL string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return publicURL, nil
	}
	for _, candidate := range strings.Split(raw, ",") {
		candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")
		if candidate == "" || candidate == "*" {
			return "", errors.New("CORS_ORIGIN must be the same origin as PUBLIC_URL in production")
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.User != nil || parsed.Host == "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") ||
			(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", errors.New("CORS_ORIGIN must contain only an absolute scheme and host")
		}
		origin := parsed.Scheme + "://" + parsed.Host
		if !strings.EqualFold(origin, publicURL) {
			return "", errors.New("CORS_ORIGIN must match PUBLIC_URL in production")
		}
	}
	// identity workspace is served from one origin; avoid echoing duplicate entries.
	return publicURL, nil
}

func validatePublicURL(raw string, production bool) (string, error) {
	if raw == "" {
		if production {
			return "", errors.New("PUBLIC_URL is required in production")
		}
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("PUBLIC_URL must be an absolute http(s) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("PUBLIC_URL must contain only scheme and host, without credentials, path, query, or fragment")
	}
	if production && parsed.Scheme != "https" {
		return "", errors.New("PUBLIC_URL must use HTTPS in production")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func validateCallbackURL(raw, publicURL, expectedPath string, production bool) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("must be an absolute http(s) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != expectedPath {
		return fmt.Errorf("must use the exact path %s and have no credentials, query, or fragment", expectedPath)
	}
	if production {
		if parsed.Scheme != "https" {
			return errors.New("must use HTTPS in production")
		}
		if parsed.Scheme+"://"+parsed.Host != publicURL {
			return errors.New("must use the same origin as PUBLIC_URL")
		}
	}
	return nil
}

func usesDevelopmentDatabaseCredentials(dsn string) bool {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.User == nil {
		return dsn == defaultDevelopmentDSN
	}
	password, _ := parsed.User.Password()
	return parsed.User.Username() == "identity" && password == "identity-local-only"
}

func secretEnv(key string) (string, error) {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return strings.TrimRight(value, "\r\n"), nil
	}
	filePath := strings.TrimSpace(os.Getenv(key + "_FILE"))
	if filePath == "" {
		return "", nil
	}
	body, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", key, err)
	}
	return strings.TrimRight(string(body), "\r\n"), nil
}

func envBool(key string, fallback bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", key)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func environmentName(production bool) string {
	if production {
		return "production"
	}
	return "development"
}
