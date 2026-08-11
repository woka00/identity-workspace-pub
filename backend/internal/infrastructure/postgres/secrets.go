package postgres

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	encryptedSecretPrefixV1 = "enc:v1:"
	encryptedSecretPrefix   = "enc:v2:"
)

type SecretCipher struct {
	aead cipher.AEAD
}

// NewSecretCipher accepts a base64-encoded 32-byte AES-256 key.
func NewSecretCipher(encodedKey string) (*SecretCipher, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return nil, nil
	}
	var key []byte
	var err error
	for _, decoder := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		key, err = decoder.DecodeString(encodedKey)
		if err == nil {
			break
		}
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("DATA_ENCRYPTION_KEY must be base64 for exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create secret cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create secret AEAD: %w", err)
	}
	return &SecretCipher{aead: aead}, nil
}

func (c *SecretCipher) Encrypt(plaintext string) (string, error) {
	return c.EncryptFor("generic", plaintext)
}

func (c *SecretCipher) EncryptFor(purpose, plaintext string) (string, error) {
	if plaintext == "" || c == nil {
		return plaintext, nil
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return "", errors.New("secret encryption purpose is required")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate secret nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), secretAAD(purpose))
	payload := append(nonce, sealed...)
	return encryptedSecretPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (c *SecretCipher) Decrypt(value string) (string, error) {
	return c.DecryptFor("generic", value)
}

func (c *SecretCipher) DecryptFor(purpose, value string) (string, error) {
	if value == "" || !isEncryptedSecret(value) {
		// Legacy plaintext remains readable and is re-encrypted during startup.
		return value, nil
	}
	if c == nil {
		return "", errors.New("encrypted database secret cannot be read without DATA_ENCRYPTION_KEY")
	}
	prefix := encryptedSecretPrefix
	aad := secretAAD(strings.TrimSpace(purpose))
	if strings.HasPrefix(value, encryptedSecretPrefixV1) {
		prefix = encryptedSecretPrefixV1
		aad = []byte(encryptedSecretPrefixV1)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil || len(payload) < c.aead.NonceSize()+c.aead.Overhead() {
		return "", errors.New("encrypted database secret is malformed")
	}
	nonce := payload[:c.aead.NonceSize()]
	ciphertext := payload[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", errors.New("encrypted database secret authentication failed")
	}
	return string(plaintext), nil
}

func secretAAD(purpose string) []byte {
	return []byte("identity-workspace-secret:v2:" + purpose)
}

func isEncryptedSecret(value string) bool {
	return strings.HasPrefix(value, encryptedSecretPrefix) || strings.HasPrefix(value, encryptedSecretPrefixV1)
}

func isCurrentEncryptedSecret(value string) bool {
	return strings.HasPrefix(value, encryptedSecretPrefix)
}

func (s *Repository) encryptSecret(purpose, value string) (string, error) {
	if s.secretCipher == nil {
		return value, nil
	}
	return s.secretCipher.EncryptFor(purpose, value)
}

func (s *Repository) decryptSecret(purpose, value string) (string, error) {
	if !isEncryptedSecret(value) {
		return value, nil
	}
	if s.secretCipher == nil {
		return "", errors.New("database contains encrypted OAuth tokens but DATA_ENCRYPTION_KEY is missing")
	}
	return s.secretCipher.DecryptFor(purpose, value)
}

func (s *Repository) upgradeSecret(purpose, value string) (string, error) {
	plaintext, err := s.decryptSecret(purpose, value)
	if err != nil {
		// Version 1 ciphertext used fixed associated data, so it remains readable
		// through DecryptFor. A failure here means a wrong key or corrupted data.
		return "", err
	}
	if isCurrentEncryptedSecret(value) {
		return value, nil
	}
	return s.secretCipher.EncryptFor(purpose, plaintext)
}

func fatSecretTokenPurpose(userID int64) string {
	return fmt.Sprintf("fatsecret:connection:%d:token", userID)
}

func fatSecretSecretPurpose(userID int64) string {
	return fmt.Sprintf("fatsecret:connection:%d:secret", userID)
}

func fatSecretRequestPurpose(oauthToken string) string {
	return "fatsecret:request:" + oauthToken + ":secret"
}

func tickTickTokenPurpose(userID int64) string {
	return fmt.Sprintf("ticktick:connection:%d:token", userID)
}

// ReencryptLegacySecrets upgrades existing plaintext OAuth credentials in place.
func (s *Repository) ReencryptLegacySecrets(ctx context.Context) error {
	if s.secretCipher == nil {
		return nil
	}

	type fatSecretConnection struct {
		userID int64
		token  string
		secret string
	}
	type fatSecretRequest struct {
		token  string
		secret string
	}
	type tickTickConnection struct {
		userID int64
		token  string
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var fatSecretConnections []fatSecretConnection
	rows, err := tx.QueryContext(ctx, `SELECT user_id, oauth_token, oauth_token_secret FROM user_fatsecret_connections FOR UPDATE`)
	if err != nil {
		return fmt.Errorf("read FatSecret connections: %w", err)
	}
	for rows.Next() {
		var row fatSecretConnection
		if err := rows.Scan(&row.userID, &row.token, &row.secret); err != nil {
			rows.Close()
			return err
		}
		fatSecretConnections = append(fatSecretConnections, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	var fatSecretRequests []fatSecretRequest
	rows, err = tx.QueryContext(ctx, `SELECT oauth_token, oauth_token_secret FROM user_fatsecret_oauth_requests FOR UPDATE`)
	if err != nil {
		return fmt.Errorf("read FatSecret OAuth requests: %w", err)
	}
	for rows.Next() {
		var row fatSecretRequest
		if err := rows.Scan(&row.token, &row.secret); err != nil {
			rows.Close()
			return err
		}
		fatSecretRequests = append(fatSecretRequests, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	var tickTickConnections []tickTickConnection
	rows, err = tx.QueryContext(ctx, `SELECT user_id, access_token FROM user_ticktick_connections FOR UPDATE`)
	if err != nil {
		return fmt.Errorf("read TickTick connections: %w", err)
	}
	for rows.Next() {
		var row tickTickConnection
		if err := rows.Scan(&row.userID, &row.token); err != nil {
			rows.Close()
			return err
		}
		tickTickConnections = append(tickTickConnections, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, row := range fatSecretConnections {
		tokenPurpose := fatSecretTokenPurpose(row.userID)
		secretPurpose := fatSecretSecretPurpose(row.userID)
		encToken, err := s.upgradeSecret(tokenPurpose, row.token)
		if err != nil {
			return fmt.Errorf("decrypt FatSecret token for user %d: %w", row.userID, err)
		}
		encSecret, err := s.upgradeSecret(secretPurpose, row.secret)
		if err != nil {
			return fmt.Errorf("decrypt FatSecret secret for user %d: %w", row.userID, err)
		}
		if encToken == row.token && encSecret == row.secret {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE user_fatsecret_connections SET oauth_token=$2, oauth_token_secret=$3 WHERE user_id=$1`, row.userID, encToken, encSecret); err != nil {
			return err
		}
	}
	for _, row := range fatSecretRequests {
		purpose := fatSecretRequestPurpose(row.token)
		encSecret, err := s.upgradeSecret(purpose, row.secret)
		if err != nil {
			return fmt.Errorf("decrypt FatSecret OAuth request: %w", err)
		}
		if encSecret == row.secret {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE user_fatsecret_oauth_requests SET oauth_token_secret=$2 WHERE oauth_token=$1`, row.token, encSecret); err != nil {
			return err
		}
	}
	for _, row := range tickTickConnections {
		purpose := tickTickTokenPurpose(row.userID)
		encToken, err := s.upgradeSecret(purpose, row.token)
		if err != nil {
			return fmt.Errorf("decrypt TickTick token for user %d: %w", row.userID, err)
		}
		if encToken == row.token {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE user_ticktick_connections SET access_token=$2 WHERE user_id=$1`, row.userID, encToken); err != nil {
			return err
		}
	}
	return tx.Commit()
}
