package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/infrastructure/brevo"
	fatsecretinfra "avatar-id/internal/infrastructure/fatsecret"
	githubinfra "avatar-id/internal/infrastructure/github"
	openfoodfactsinfra "avatar-id/internal/infrastructure/openfoodfacts"
	"avatar-id/internal/infrastructure/postgres"
	webpushinfra "avatar-id/internal/infrastructure/webpush"
	"avatar-id/internal/transport/httpapi"
)

type serverRuntime struct {
	server   *http.Server
	service  *application.Service
	location *time.Location
}

func newServerRuntime(cfg runtimeConfig, repository *postgres.Repository, logs httpapi.LogSource) (serverRuntime, error) {
	location, err := time.LoadLocation(cfg.playerTZ)
	if err != nil {
		return serverRuntime{}, fmt.Errorf("bad PLAYER_TZ: %w", err)
	}
	now := func() time.Time { return time.Now().In(location) }

	fatSecretClient := fatsecretinfra.NewClient(
		cfg.fatSecretKey, cfg.fatSecretSecret, cfg.fatSecretRegion, cfg.fatSecretLanguage,
	)
	pushClient, err := webpushinfra.New(cfg.vapidPrivateKey, cfg.dataEncryptionKey, cfg.vapidSubject)
	if err != nil {
		return serverRuntime{}, fmt.Errorf("Web Push configuration: %w", err)
	}
	githubClient := githubinfra.NewClient()
	foodCatalogClient := openfoodfactsinfra.NewClient(cfg.openFoodFactsUserAgent)
	service := application.New(repository, fatSecretClient, now).
		WithPush(pushClient).
		WithGitHub(githubClient).
		WithFoodCatalog(foodCatalogClient)
	if cfg.registrationEnabled {
		mailer, err := brevo.New(cfg.brevoAPIKey, cfg.brevoSenderEmail, cfg.brevoSenderName)
		if err != nil {
			return serverRuntime{}, err
		}
		registration, err := application.NewRegistration(repository, mailer, cfg.publicURL)
		if err != nil {
			return serverRuntime{}, err
		}
		service.WithRegistration(registration)
	}
	handler := httpapi.New(service, httpapi.Config{
		StaticDir:            cfg.staticDir,
		CORSOrigin:           cfg.corsOrigin,
		FatSecretCallbackURL: cfg.fatSecretCallbackURL,
		PublicURL:            cfg.publicURL,
		Production:           cfg.production,
		TrustProxy:           cfg.trustProxy,
		SecureCookies:        cfg.production || strings.HasPrefix(cfg.publicURL, "https://"),
		Logs:                 logs,
	})

	return serverRuntime{
		server: &http.Server{
			Addr:              cfg.addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       2 * time.Minute,
			WriteTimeout:      2 * time.Minute,
			IdleTimeout:       75 * time.Second,
			MaxHeaderBytes:    32 << 10,
		},
		service:  service,
		location: location,
	}, nil
}
