package webpush

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestClientDerivesStableVAPIDKey(t *testing.T) {
	first, err := New("", "stable-encryption-secret", "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := New("", "stable-encryption-secret", "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Configured() || first.PublicKey() == "" || first.PublicKey() != second.PublicKey() {
		t.Fatalf("VAPID key is not stable: %q / %q", first.PublicKey(), second.PublicKey())
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first.PublicKey())
	if err != nil || len(decoded) != 65 || decoded[0] != 4 {
		t.Fatalf("unexpected public key: length=%d err=%v", len(decoded), err)
	}
}

func TestEncryptPayloadProducesAES128GCMRecord(t *testing.T) {
	receiver, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	body, err := encryptPayload(
		[]byte(`{"title":"Напоминание"}`),
		base64.RawURLEncoding.EncodeToString(receiver.PublicKey().Bytes()),
		base64.RawURLEncoding.EncodeToString(auth),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= 86 || body[20] != 65 {
		t.Fatalf("unexpected encrypted record: length=%d keyLength=%d", len(body), body[20])
	}
}

func TestClientRejectsInvalidConfiguration(t *testing.T) {
	if _, err := New("not-base64url", "", "mailto:test@example.com"); err == nil {
		t.Fatal("expected invalid private key error")
	}
	if _, err := New("", "secret", "http://example.com"); err == nil {
		t.Fatal("expected invalid VAPID subject error")
	}
}
