package main

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

type foodCatalogImporter func(context.Context, io.Reader) (read int64, upserted int64, err error)

func runAdminCommand(accounts *application.AccountAdministration, importCatalog foodCatalogImporter, command string, args []string) error {
	timeout := 2 * time.Minute
	if command == "import-food-catalog" {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch command {
	case "migrate":
		log.Print("migrations applied successfully")
		return nil
	case "set-password":
		if len(args) != 1 {
			return errors.New("usage: avatar-id-server set-password <login>; provide AVATAR_NEW_PASSWORD or AVATAR_NEW_PASSWORD_FILE")
		}
		password, err := secretEnv("AVATAR_NEW_PASSWORD")
		if err != nil {
			return err
		}
		if password == "" {
			return errors.New("AVATAR_NEW_PASSWORD or AVATAR_NEW_PASSWORD_FILE is required")
		}
		if err := accounts.SetPassword(ctx, args[0], password); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("user %q does not exist", args[0])
			}
			return err
		}
		log.Printf("password replaced and all sessions revoked for %s", args[0])
		return nil
	case "disable-user", "enable-user":
		if len(args) != 1 {
			return fmt.Errorf("usage: avatar-id-server %s <login>", command)
		}
		enabled := command == "enable-user"
		if err := accounts.SetUserEnabled(ctx, args[0], enabled); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("user %q does not exist", args[0])
			}
			return err
		}
		log.Printf("user %s enabled=%t", args[0], enabled)
		return nil
	case "grant-admin", "revoke-admin":
		if len(args) != 1 {
			return fmt.Errorf("usage: avatar-id-server %s <login>", command)
		}
		enabled := command == "grant-admin"
		if err := accounts.SetUserAdmin(ctx, args[0], enabled); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("user %q does not exist", args[0])
			}
			return err
		}
		log.Printf("user %s admin=%t", args[0], enabled)
		return nil
	case "security-status", "predeploy":
		users, err := accounts.UnrotatedEnabledUsers(ctx)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			if command == "predeploy" {
				log.Print("migrations applied and all enabled users have production passwords")
			} else {
				log.Print("all enabled users have production passwords")
			}
			return nil
		}
		return fmt.Errorf("enabled users still using unconfirmed preview passwords: %s", strings.Join(users, ", "))
	case "import-food-catalog":
		if len(args) != 1 {
			return errors.New("usage: avatar-id-server import-food-catalog <catalog.jsonl[.gz]>")
		}
		file, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer file.Close()
		var input io.Reader = file
		if strings.HasSuffix(strings.ToLower(args[0]), ".gz") {
			compressed, err := gzip.NewReader(file)
			if err != nil {
				return err
			}
			defer compressed.Close()
			input = compressed
		}
		read, upserted, err := importCatalog(ctx, input)
		if err != nil {
			return err
		}
		log.Printf("food catalog imported: read=%d upserted=%d", read, upserted)
		return nil
	default:
		return errors.New("unknown command; supported: serve, migrate, predeploy, set-password, enable-user, disable-user, grant-admin, revoke-admin, security-status, import-food-catalog")
	}
}
