package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/infrastructure/postgres"
	"avatar-id/internal/transport/httpapi"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	runtimeLogs := httpapi.NewLogBuffer(1000)
	log.SetOutput(io.MultiWriter(os.Stderr, runtimeLogs))

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	db, err := openDatabase(cfg.databaseURL, cfg.databaseMaxOpenConns, cfg.databaseMaxIdleConns)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	cipher, err := postgres.NewSecretCipher(cfg.dataEncryptionKey)
	if err != nil {
		log.Fatalf("DATA_ENCRYPTION_KEY: %v", err)
	}
	repository := postgres.New(db, cipher)
	accounts := application.NewAccountAdministration(repository)

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
		importCatalog := func(ctx context.Context, input io.Reader) (int64, int64, error) {
			stats, err := repository.ImportFoodCatalogJSONL(ctx, input)
			return stats.Read, stats.Upserted, err
		}
		if err := runAdminCommand(accounts, importCatalog, command, os.Args[2:]); err != nil {
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
		unrotated, err := accounts.UnrotatedEnabledUsers(ctx)
		cancel()
		if err != nil {
			log.Fatalf("check production accounts: %v", err)
		}
		if len(unrotated) > 0 {
			log.Fatalf("production startup blocked: replace preview passwords or disable these accounts first: %s; see PRODUCTION_DEPLOYMENT_RU.md", strings.Join(unrotated, ", "))
		}
	}

	runtime, err := newServerRuntime(cfg, repository, runtimeLogs)
	if err != nil {
		log.Fatal(err)
	}

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go runtime.service.RunReminderWorker(workerCtx)

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("identity workspace listening on %s (environment=%s, calendar timezone=%s)", cfg.addr, environmentName(cfg.production), runtime.location)
		serverErrors <- runtime.server.ListenAndServe()
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
	if err := runtime.server.Shutdown(shutdownCtx); err != nil {
		_ = runtime.server.Close()
		log.Fatalf("graceful shutdown: %v", err)
	}
}
