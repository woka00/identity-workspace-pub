package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"avatar-id/internal/application"
)

func userEmailPurpose(userID int64) string { return fmt.Sprintf("user:%d:email", userID) }

func (s *Repository) PrepareRegistration(ctx context.Context, email, tokenHash string, now, expires time.Time) (bool, error) {
	fingerprint, err := s.secretCipher.emailFingerprint(email)
	if err != nil {
		return false, err
	}
	ciphertext, err := s.secretCipher.EncryptFor("registration:email:"+fingerprint, email)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// One short lock makes the durable quotas atomic across all server replicas.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(724830145)`); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM registration_mail_attempts WHERE attempted_at <= $1`, now.Add(-24*time.Hour)); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM registration_requests WHERE expires_at <= $1`, now); err != nil {
		return false, err
	}
	var daily, hourly, perEmail int
	var cooldown bool
	err = tx.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE attempted_at > $2),
        count(*) FILTER (WHERE email_hash=$1 AND attempted_at > $2),
        COALESCE(bool_or(email_hash=$1 AND attempted_at > $3), false)
		FROM registration_mail_attempts`, fingerprint, now.Add(-time.Hour), now.Add(-application.RegistrationEmailCooldown)).Scan(&daily, &hourly, &perEmail, &cooldown)
	if err != nil {
		return false, err
	}
	if daily >= application.RegistrationGlobalDailyLimit || hourly >= application.RegistrationGlobalHourlyLimit || perEmail >= application.RegistrationEmailHourlyLimit || cooldown {
		return false, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO registration_mail_attempts (email_hash, attempted_at) VALUES ($1,$2)`, fingerprint, now); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO registration_requests (email_hash,email_encrypted,token_hash,expires_at)
        VALUES ($1,$2,$3,$4) ON CONFLICT (email_hash) DO UPDATE SET
        email_encrypted=EXCLUDED.email_encrypted,token_hash=EXCLUDED.token_hash,expires_at=EXCLUDED.expires_at`, fingerprint, ciphertext, tokenHash, expires)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Repository) RegistrationTokenValid(ctx context.Context, tokenHash string, now time.Time) (bool, error) {
	var valid bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM registration_requests WHERE token_hash=$1 AND expires_at>$2)`, tokenHash, now).Scan(&valid)
	return valid, err
}

func (s *Repository) CompleteRegistration(ctx context.Context, tokenHash, login, normalized, passwordHash string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var fingerprint, encryptedEmail string
	err = tx.QueryRowContext(ctx, `SELECT email_hash,email_encrypted FROM registration_requests WHERE token_hash=$1 AND expires_at>$2 FOR UPDATE`, tokenHash, now).Scan(&fingerprint, &encryptedEmail)
	if err == sql.ErrNoRows {
		return application.ErrRegistrationToken
	}
	if err != nil {
		return err
	}
	email, err := s.secretCipher.DecryptFor("registration:email:"+fingerprint, encryptedEmail)
	if err != nil {
		return err
	}
	// Verify the blind index as well as AEAD before creating a verified account.
	actual, err := s.secretCipher.emailFingerprint(email)
	if err != nil {
		return err
	}
	if actual != fingerprint {
		return application.ErrRegistrationToken
	}
	var userID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO users (login,login_normalized,password_hash,is_enabled,is_admin,password_rotated_at,email_hash,email_encrypted,email_verified_at)
        VALUES ($1,$2,$3,TRUE,FALSE,$4,$5,$6,$4) ON CONFLICT DO NOTHING RETURNING id`, login, normalized, passwordHash, now, fingerprint, encryptedEmail).Scan(&userID)
	if err == sql.ErrNoRows {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email_hash=$1)`, fingerprint).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return application.ErrRegistrationEmail
		}
		return application.ErrRegistrationLogin
	}
	if err != nil {
		return err
	}
	// Bind persisted email ciphertext to its owning account, not only its index.
	accountEmail, err := s.secretCipher.EncryptFor(userEmailPurpose(userID), email)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET email_encrypted=$2 WHERE id=$1`, userID, accountEmail); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_profiles (user_id) VALUES ($1)`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_tracker_settings (user_id) VALUES ($1)`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_task_categories (user_id,name,name_normalized)
        VALUES ($1,'Дом','дом'),($1,'Работа','работа')`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM registration_requests WHERE email_hash=$1`, fingerprint); err != nil {
		return err
	}
	return tx.Commit()
}
