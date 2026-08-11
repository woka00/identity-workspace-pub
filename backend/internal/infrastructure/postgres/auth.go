package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"identity-workspace/internal/domain"
)

func currentUserID(ctx context.Context) (int64, error) {
	userID, err := domain.UserID(ctx)
	if err != nil {
		return 0, fmt.Errorf("authentication required: %w", domain.ErrUnauthorized)
	}
	return userID, nil
}

func (s *Repository) UserByLogin(ctx context.Context, normalized string) (domain.UserCredential, error) {
	var credential domain.UserCredential
	err := s.db.QueryRowContext(ctx, `
		SELECT id, login, to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), password_hash
		FROM users WHERE login_normalized=$1 AND is_enabled=TRUE`, normalized).Scan(
		&credential.ID, &credential.Login, &credential.CreatedAt, &credential.PasswordHash,
	)
	if err == sql.ErrNoRows {
		return domain.UserCredential{}, domain.ErrNotFound
	}
	return credential, err
}

func (s *Repository) UpdatePasswordHash(ctx context.Context, userID int64, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET password_hash=$2 WHERE id=$1 AND is_enabled=TRUE`, userID, passwordHash)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Repository) AdminSetPassword(ctx context.Context, normalizedLogin, passwordHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID int64
	err = tx.QueryRowContext(ctx, `
		UPDATE users
		SET password_hash=$2, password_rotated_at=now()
		WHERE login_normalized=$1
		RETURNING id`, normalizedLogin, passwordHash).Scan(&userID)
	if err == sql.ErrNoRows {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE user_id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) AdminSetUserEnabled(ctx context.Context, normalizedLogin string, enabled bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID int64
	err = tx.QueryRowContext(ctx, `
		UPDATE users SET is_enabled=$2
		WHERE login_normalized=$1
		RETURNING id`, normalizedLogin, enabled).Scan(&userID)
	if err == sql.ErrNoRows {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !enabled {
		if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE user_id=$1`, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Repository) UnrotatedEnabledUsers(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT login FROM users
		WHERE is_enabled=TRUE AND password_rotated_at IS NULL
		ORDER BY login_normalized`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		out = append(out, login)
	}
	return out, rows.Err()
}

func (s *Repository) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at <= $1`, time.Now()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO auth_sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`, userID, tokenHash, expiresAt); err != nil {
		return err
	}
	// Limit the damage of copied credentials and keep the session table bounded.
	// The ten most recently used sessions remain valid for this account.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM auth_sessions
		WHERE user_id=$1 AND id NOT IN (
			SELECT id FROM auth_sessions
			WHERE user_id=$1
			ORDER BY last_seen_at DESC, id DESC
			LIMIT 10
		)`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) UserBySession(ctx context.Context, tokenHash string, now time.Time) (domain.User, error) {
	var user domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT account.id, account.login,
		       to_char(account.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
		FROM auth_sessions AS session
		JOIN users AS account ON account.id=session.user_id
		WHERE session.token_hash=$1
		  AND session.expires_at>$2
		  AND session.last_seen_at>$3
		  AND account.is_enabled=TRUE`,
		tokenHash, now, now.Add(-24*time.Hour),
	).Scan(&user.ID, &user.Login, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return domain.User{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.User{}, err
	}
	_, _ = s.db.ExecContext(ctx, `
		UPDATE auth_sessions SET last_seen_at=$2
		WHERE token_hash=$1 AND last_seen_at<$3`, tokenHash, now, now.Add(-5*time.Minute))
	return user, nil
}

func (s *Repository) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash=$1`, tokenHash)
	return err
}
