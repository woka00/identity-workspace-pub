package application

import "context"

type AccountAdministrationRepository interface {
	AdminSetPassword(context.Context, string, string) error
	AdminSetUserEnabled(context.Context, string, bool) error
	AdminSetUserAdmin(context.Context, string, bool) error
	UnrotatedEnabledUsers(context.Context) ([]string, error)
}

type AccountAdministration struct {
	repository AccountAdministrationRepository
}

func NewAccountAdministration(repository AccountAdministrationRepository) *AccountAdministration {
	return &AccountAdministration{repository: repository}
}

func (administration *AccountAdministration) SetPassword(ctx context.Context, login, password string) error {
	_, normalized, err := normalizeLogin(login)
	if err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return administration.repository.AdminSetPassword(ctx, normalized, hash)
}

func (administration *AccountAdministration) SetUserEnabled(ctx context.Context, login string, enabled bool) error {
	_, normalized, err := normalizeLogin(login)
	if err != nil {
		return err
	}
	return administration.repository.AdminSetUserEnabled(ctx, normalized, enabled)
}

func (administration *AccountAdministration) SetUserAdmin(ctx context.Context, login string, enabled bool) error {
	_, normalized, err := normalizeLogin(login)
	if err != nil {
		return err
	}
	return administration.repository.AdminSetUserAdmin(ctx, normalized, enabled)
}

func (administration *AccountAdministration) UnrotatedEnabledUsers(ctx context.Context) ([]string, error) {
	return administration.repository.UnrotatedEnabledUsers(ctx)
}
