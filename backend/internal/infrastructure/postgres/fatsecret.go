package postgres

import (
	"context"
	"database/sql"
	"errors"

	"identity-workspace/internal/domain"
)

func (s *Repository) FatSecretConnection(ctx context.Context) (domain.FatSecretConnection, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.FatSecretConnection{}, err
	}
	var connection domain.FatSecretConnection
	err = s.db.QueryRowContext(ctx, `
        SELECT oauth_token, oauth_token_secret,
               to_char(connected_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
        FROM user_fatsecret_connections
        WHERE user_id = $1`, userID).Scan(
		&connection.OAuthToken,
		&connection.OAuthTokenSecret,
		&connection.ConnectedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FatSecretConnection{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FatSecretConnection{}, err
	}
	connection.OAuthToken, err = s.decryptSecret(fatSecretTokenPurpose(userID), connection.OAuthToken)
	if err != nil {
		return domain.FatSecretConnection{}, err
	}
	connection.OAuthTokenSecret, err = s.decryptSecret(fatSecretSecretPurpose(userID), connection.OAuthTokenSecret)
	if err != nil {
		return domain.FatSecretConnection{}, err
	}
	return connection, nil
}

func (s *Repository) SaveFatSecretOAuthRequest(ctx context.Context, request domain.FatSecretOAuthRequest) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	encryptedSecret, err := s.encryptSecret(fatSecretRequestPurpose(request.OAuthToken), request.OAuthTokenSecret)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
        DELETE FROM user_fatsecret_oauth_requests
        WHERE created_at < now() - interval '30 minutes'`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
        INSERT INTO user_fatsecret_oauth_requests (
            oauth_token, user_id, oauth_token_secret, return_to
        ) VALUES ($1, $2, $3, $4)
        ON CONFLICT (oauth_token) DO UPDATE
        SET user_id = EXCLUDED.user_id,
            oauth_token_secret = EXCLUDED.oauth_token_secret,
            return_to = EXCLUDED.return_to,
            created_at = now()`,
		request.OAuthToken, userID, encryptedSecret, request.ReturnTo,
	)
	return err
}

func (s *Repository) ConsumeFatSecretOAuthRequest(ctx context.Context, token string) (domain.FatSecretOAuthRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.FatSecretOAuthRequest{}, err
	}
	defer tx.Rollback()

	var request domain.FatSecretOAuthRequest
	err = tx.QueryRowContext(ctx, `
        SELECT user_id, oauth_token, oauth_token_secret, return_to
        FROM user_fatsecret_oauth_requests
        WHERE oauth_token = $1
          AND created_at >= now() - interval '30 minutes'
        FOR UPDATE`, token).Scan(
		&request.UserID,
		&request.OAuthToken,
		&request.OAuthTokenSecret,
		&request.ReturnTo,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FatSecretOAuthRequest{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FatSecretOAuthRequest{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM user_fatsecret_oauth_requests WHERE oauth_token = $1`, token,
	); err != nil {
		return domain.FatSecretOAuthRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.FatSecretOAuthRequest{}, err
	}
	request.OAuthTokenSecret, err = s.decryptSecret(fatSecretRequestPurpose(request.OAuthToken), request.OAuthTokenSecret)
	if err != nil {
		return domain.FatSecretOAuthRequest{}, err
	}
	return request, nil
}

func (s *Repository) SaveFatSecretConnection(ctx context.Context, userID int64, token, secret string) error {
	encryptedToken, err := s.encryptSecret(fatSecretTokenPurpose(userID), token)
	if err != nil {
		return err
	}
	encryptedSecret, err := s.encryptSecret(fatSecretSecretPurpose(userID), secret)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
        INSERT INTO user_fatsecret_connections (user_id, oauth_token, oauth_token_secret)
        VALUES ($1, $2, $3)
        ON CONFLICT (user_id) DO UPDATE
        SET oauth_token = EXCLUDED.oauth_token,
            oauth_token_secret = EXCLUDED.oauth_token_secret,
            connected_at = now()`, userID, encryptedToken, encryptedSecret)
	return err
}

func (s *Repository) DeleteFatSecretConnection(ctx context.Context) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM user_fatsecret_connections WHERE user_id = $1`, userID)
	return err
}
