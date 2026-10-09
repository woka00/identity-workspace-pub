package application

import (
	"context"

	"avatar-id/internal/domain"
)

// Request identity stays in the application layer so the domain model does not
// depend on transport details such as context propagation.
type principalContextKey struct{}

type principal struct {
	userID  int64
	isAdmin bool
}

func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal{userID: userID})
}

func WithUser(ctx context.Context, user domain.User) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal{userID: user.ID, isAdmin: user.IsAdmin})
}

func UserID(ctx context.Context) (int64, error) {
	value, ok := ctx.Value(principalContextKey{}).(principal)
	if !ok || value.userID <= 0 {
		return 0, domain.ErrUnauthorized
	}
	return value.userID, nil
}

func RequireAdmin(ctx context.Context) error {
	value, ok := ctx.Value(principalContextKey{}).(principal)
	if !ok || value.userID <= 0 || !value.isAdmin {
		return domain.ErrForbidden
	}
	return nil
}
