package application

import (
	"context"
	"testing"
)

type accountAdministrationRepositoryStub struct {
	login        string
	passwordHash string
	enabled      bool
	admin        bool
	unrotated    []string
}

func (repository *accountAdministrationRepositoryStub) AdminSetPassword(_ context.Context, login, passwordHash string) error {
	repository.login = login
	repository.passwordHash = passwordHash
	return nil
}

func (repository *accountAdministrationRepositoryStub) AdminSetUserEnabled(_ context.Context, login string, enabled bool) error {
	repository.login = login
	repository.enabled = enabled
	return nil
}

func (repository *accountAdministrationRepositoryStub) AdminSetUserAdmin(_ context.Context, login string, enabled bool) error {
	repository.login = login
	repository.admin = enabled
	return nil
}

func (repository *accountAdministrationRepositoryStub) UnrotatedEnabledUsers(context.Context) ([]string, error) {
	return repository.unrotated, nil
}

func TestAccountAdministrationSetPassword(t *testing.T) {
	repository := &accountAdministrationRepositoryStub{}
	administration := NewAccountAdministration(repository)
	password := "a-secure-password"

	if err := administration.SetPassword(context.Background(), " Avatar01 ", password); err != nil {
		t.Fatal(err)
	}
	if repository.login != "avatar01" {
		t.Fatalf("normalized login = %q", repository.login)
	}
	if !verifyPassword(repository.passwordHash, password) {
		t.Fatal("administration stored an invalid password hash")
	}
}

func TestAccountAdministrationSetUserEnabled(t *testing.T) {
	repository := &accountAdministrationRepositoryStub{}
	administration := NewAccountAdministration(repository)

	if err := administration.SetUserEnabled(context.Background(), " Avatar02 ", true); err != nil {
		t.Fatal(err)
	}
	if repository.login != "avatar02" || !repository.enabled {
		t.Fatalf("update = login %q, enabled %t", repository.login, repository.enabled)
	}
}

func TestAccountAdministrationSetUserAdmin(t *testing.T) {
	repository := &accountAdministrationRepositoryStub{}
	administration := NewAccountAdministration(repository)

	if err := administration.SetUserAdmin(context.Background(), " Avatar01 ", true); err != nil {
		t.Fatal(err)
	}
	if repository.login != "avatar01" || !repository.admin {
		t.Fatalf("admin update = login %q, enabled %t", repository.login, repository.admin)
	}
}
