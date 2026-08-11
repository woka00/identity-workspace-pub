package postgres

import (
	"encoding/base64"
	"strings"
	"testing"
)

func testCipher(t *testing.T, fill byte) *SecretCipher {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = fill
	}
	cipher, err := NewSecretCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func TestSecretCipherRoundTrip(t *testing.T) {
	cipher := testCipher(t, 0x42)
	encrypted, err := cipher.Encrypt("oauth-access-token")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "oauth-access-token" || !strings.HasPrefix(encrypted, encryptedSecretPrefix) {
		t.Fatalf("secret was not encrypted: %q", encrypted)
	}
	plain, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "oauth-access-token" {
		t.Fatalf("unexpected plaintext %q", plain)
	}
}

func TestSecretCipherRejectsTamperingAndWrongKey(t *testing.T) {
	cipher := testCipher(t, 0x11)
	encrypted, err := cipher.Encrypt("sensitive")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encrypted, encryptedSecretPrefix))
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)/2] ^= 0x01
	tampered := encryptedSecretPrefix + base64.RawURLEncoding.EncodeToString(payload)
	if _, err := cipher.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
	if _, err := testCipher(t, 0x22).Decrypt(encrypted); err == nil {
		t.Fatal("ciphertext was accepted with the wrong key")
	}
}

func TestSecretCipherBindsCiphertextToPurpose(t *testing.T) {
	cipher := testCipher(t, 0x44)
	encrypted, err := cipher.EncryptFor("ticktick:connection:1:token", "account-one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cipher.DecryptFor("ticktick:connection:2:token", encrypted); err == nil {
		t.Fatal("ciphertext was accepted for a different user and purpose")
	}
	plain, err := cipher.DecryptFor("ticktick:connection:1:token", encrypted)
	if err != nil || plain != "account-one" {
		t.Fatalf("correct purpose failed: %q, %v", plain, err)
	}
}

func TestSecretCipherLegacyAndValidation(t *testing.T) {
	cipher := testCipher(t, 0x33)
	plain, err := cipher.Decrypt("legacy-plaintext")
	if err != nil || plain != "legacy-plaintext" {
		t.Fatalf("legacy plaintext: %q, %v", plain, err)
	}
	if _, err := NewSecretCipher(base64.StdEncoding.EncodeToString(make([]byte, 16))); err == nil {
		t.Fatal("short key was accepted")
	}
}
