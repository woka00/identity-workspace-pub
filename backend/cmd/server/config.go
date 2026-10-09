package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const defaultDevelopmentDSN = "postgres://avatar:avatar@localhost:5432/avatarid?sslmode=disable"

type runtimeConfig struct {
	registrationEnabled    bool
	brevoAPIKey            string
	brevoSenderEmail       string
	brevoSenderName        string
	production             bool
	addr                   string
	databaseURL            string
	databaseMaxOpenConns   int
	databaseMaxIdleConns   int
	publicURL              string
	corsOrigin             string
	staticDir              string
	playerTZ               string
	trustProxy             bool
	runMigrations          bool
	dataEncryptionKey      string
	fatSecretKey           string
	fatSecretSecret        string
	fatSecretRegion        string
	fatSecretLanguage      string
	fatSecretCallbackURL   string
	vapidPrivateKey        string
	vapidSubject           string
	openFoodFactsUserAgent string
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
	if production && usesPreviewDatabaseCredentials(dsn) {
		return runtimeConfig{}, errors.New("DATABASE_URL still uses the public preview credentials avatar/avatar")
	}

	dataEncryptionKey, err := secretEnv("DATA_ENCRYPTION_KEY")
	if err != nil {
		return runtimeConfig{}, err
	}
	if production && dataEncryptionKey == "" {
		return runtimeConfig{}, errors.New("DATA_ENCRYPTION_KEY is required in production")
	}
	registrationEnabled, err := envBool("REGISTRATION_ENABLED", false)
	if err != nil {
		return runtimeConfig{}, err
	}
	var brevoAPIKey string
	brevoSenderEmail := strings.TrimSpace(env("BREVO_SENDER_EMAIL", ""))
	if registrationEnabled {
		brevoAPIKey, err = secretEnv("BREVO_API_KEY")
		if err != nil {
			return runtimeConfig{}, err
		}
		if brevoAPIKey == "" || brevoSenderEmail == "" || dataEncryptionKey == "" || !strings.HasPrefix(publicURL, "https://") {
			return runtimeConfig{}, errors.New("registration requires BREVO_API_KEY, BREVO_SENDER_EMAIL, DATA_ENCRYPTION_KEY and an HTTPS PUBLIC_URL")
		}
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
	if production {
		if fatKey != "" && fatCallback == "" {
			fatCallback = publicURL + "/api/integrations/fatsecret/callback"
		}
	}
	if err := validateCallbackURL(fatCallback, publicURL, "/api/integrations/fatsecret/callback", production); err != nil {
		return runtimeConfig{}, fmt.Errorf("FATSECRET_CALLBACK_URL: %w", err)
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
	databaseMaxOpenConns, err := envInt("DB_MAX_OPEN_CONNS", 15, 1, 200)
	if err != nil {
		return runtimeConfig{}, err
	}
	databaseMaxIdleConns, err := envInt("DB_MAX_IDLE_CONNS", 5, 0, databaseMaxOpenConns)
	if err != nil {
		return runtimeConfig{}, err
	}

	return runtimeConfig{
		registrationEnabled:    registrationEnabled,
		brevoAPIKey:            brevoAPIKey,
		brevoSenderEmail:       brevoSenderEmail,
		brevoSenderName:        strings.TrimSpace(env("BREVO_SENDER_NAME", "identity workspace")),
		production:             production,
		addr:                   env("ADDR", ":8080"),
		databaseURL:            dsn,
		databaseMaxOpenConns:   databaseMaxOpenConns,
		databaseMaxIdleConns:   databaseMaxIdleConns,
		publicURL:              publicURL,
		corsOrigin:             corsOrigin,
		staticDir:              env("STATIC_DIR", "../frontend/dist"),
		playerTZ:               env("PLAYER_TZ", "Local"),
		trustProxy:             trustProxy,
		runMigrations:          runMigrations,
		dataEncryptionKey:      dataEncryptionKey,
		fatSecretKey:           strings.TrimSpace(fatKey),
		fatSecretSecret:        strings.TrimSpace(fatSecret),
		fatSecretRegion:        strings.ToUpper(strings.TrimSpace(env("FATSECRET_REGION", "RU"))),
		fatSecretLanguage:      strings.TrimSpace(env("FATSECRET_LANGUAGE", "ru")),
		fatSecretCallbackURL:   fatCallback,
		vapidPrivateKey:        strings.TrimSpace(vapidPrivateKey),
		vapidSubject:           vapidSubject,
		openFoodFactsUserAgent: strings.TrimSpace(env("OPENFOODFACTS_USER_AGENT", "identity-workspace/1.0 (admin@localhost)")),
	}, nil
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

func usesPreviewDatabaseCredentials(dsn string) bool {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.User == nil {
		return dsn == defaultDevelopmentDSN
	}
	password, _ := parsed.User.Password()
	return parsed.User.Username() == "avatar" && password == "avatar"
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

func envInt(key string, fallback, minimum, maximum int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, minimum, maximum)
	}
	return value, nil
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
