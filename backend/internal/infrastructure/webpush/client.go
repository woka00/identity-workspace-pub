package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"identity-workspace/internal/domain"
)

const maxPayloadBytes = 3000

var rawURL = base64.RawURLEncoding

type Client struct {
	privateKey *ecdsa.PrivateKey
	publicKey  string
	subject    string
	httpClient *http.Client
}

func New(privateKeyValue, derivationSecret, subject string) (*Client, error) {
	privateBytes, err := privateKeyBytes(privateKeyValue, derivationSecret)
	if err != nil {
		return nil, err
	}
	if len(privateBytes) == 0 {
		return &Client{}, nil
	}
	curve := elliptic.P256()
	n := curve.Params().N
	d := new(big.Int).SetBytes(privateBytes)
	d.Mod(d, new(big.Int).Sub(n, big.NewInt(1)))
	d.Add(d, big.NewInt(1))
	x, y := curve.ScalarBaseMult(paddedScalar(d, 32))
	key := &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y}, D: d}
	public := elliptic.Marshal(curve, x, y)

	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "mailto:admin@localhost"
	}
	if !strings.HasPrefix(subject, "mailto:") {
		parsed, parseErr := url.Parse(subject)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, errors.New("VAPID_SUBJECT must be mailto:... or an https URL")
		}
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           safeDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &Client{
		privateKey: key,
		publicKey:  rawURL.EncodeToString(public),
		subject:    subject,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   20 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func privateKeyBytes(value, derivationSecret string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		decoded, err := rawURL.DecodeString(value)
		if err != nil || len(decoded) != 32 {
			return nil, errors.New("VAPID_PRIVATE_KEY must be a 32-byte base64url value")
		}
		return decoded, nil
	}
	if strings.TrimSpace(derivationSecret) == "" {
		return nil, nil
	}
	mac := hmac.New(sha256.New, []byte(derivationSecret))
	_, _ = mac.Write([]byte("Identity Workspace/VAPID/P-256/v1"))
	return mac.Sum(nil), nil
}

func (c *Client) Configured() bool { return c != nil && c.privateKey != nil && c.httpClient != nil }
func (c *Client) PublicKey() string {
	if !c.Configured() {
		return ""
	}
	return c.publicKey
}

func (c *Client) Send(ctx context.Context, subscription domain.PushSubscription, payload []byte) (int, error) {
	if !c.Configured() {
		return 0, errors.New("web push is not configured")
	}
	if len(payload) == 0 || len(payload) > maxPayloadBytes {
		return 0, errors.New("web push payload has invalid size")
	}
	endpoint, err := url.Parse(subscription.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil {
		return 0, errors.New("invalid push endpoint")
	}
	body, err := encryptPayload(payload, subscription.P256DH, subscription.Auth)
	if err != nil {
		return 0, err
	}
	token, err := c.vapidToken(endpoint.Scheme + "://" + endpoint.Host)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", "vapid t="+token+", k="+c.publicKey)

	response, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("push service returned %s", response.Status)
	}
	return response.StatusCode, nil
}

func (c *Client) vapidToken(audience string) (string, error) {
	header, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "ES256"})
	claims, _ := json.Marshal(map[string]any{
		"aud": audience,
		"exp": time.Now().Add(11 * time.Hour).Unix(),
		"sub": c.subject,
	})
	unsigned := rawURL.EncodeToString(header) + "." + rawURL.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, c.privateKey, digest[:])
	if err != nil {
		return "", err
	}
	signature := append(paddedScalar(r, 32), paddedScalar(s, 32)...)
	return unsigned + "." + rawURL.EncodeToString(signature), nil
}

func encryptPayload(payload []byte, receiverKeyValue, authValue string) ([]byte, error) {
	receiverPublicBytes, err := rawURL.DecodeString(strings.TrimSpace(receiverKeyValue))
	if err != nil || len(receiverPublicBytes) != 65 || receiverPublicBytes[0] != 4 {
		return nil, errors.New("invalid push p256dh key")
	}
	authSecret, err := rawURL.DecodeString(strings.TrimSpace(authValue))
	if err != nil || len(authSecret) < 16 {
		return nil, errors.New("invalid push auth key")
	}
	curve := ecdh.P256()
	receiverPublic, err := curve.NewPublicKey(receiverPublicBytes)
	if err != nil {
		return nil, errors.New("invalid push public key")
	}
	senderPrivate, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	senderPublicBytes := senderPrivate.PublicKey().Bytes()
	sharedSecret, err := senderPrivate.ECDH(receiverPublic)
	if err != nil {
		return nil, err
	}

	prkKey := hkdfExtract(authSecret, sharedSecret)
	keyInfo := append([]byte("WebPush: info\x00"), receiverPublicBytes...)
	keyInfo = append(keyInfo, senderPublicBytes...)
	ikm := hkdfExpand(prkKey, keyInfo, 32)

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	prk := hkdfExtract(salt, ikm)
	contentKey := hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce := hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)
	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext := append(append([]byte(nil), payload...), 0x02)
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	const recordSize = 4096
	body := make([]byte, 0, 16+4+1+len(senderPublicBytes)+len(ciphertext))
	body = append(body, salt...)
	record := make([]byte, 4)
	binary.BigEndian.PutUint32(record, recordSize)
	body = append(body, record...)
	body = append(body, byte(len(senderPublicBytes)))
	body = append(body, senderPublicBytes...)
	body = append(body, ciphertext...)
	return body, nil
}

func hkdfExtract(salt, input []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	mac := hmac.New(sha256.New, salt)
	_, _ = mac.Write(input)
	return mac.Sum(nil)
}

func hkdfExpand(prk, info []byte, length int) []byte {
	result := make([]byte, 0, length)
	var previous []byte
	for counter := byte(1); len(result) < length; counter++ {
		mac := hmac.New(sha256.New, prk)
		_, _ = mac.Write(previous)
		_, _ = mac.Write(info)
		_, _ = mac.Write([]byte{counter})
		previous = mac.Sum(nil)
		result = append(result, previous...)
	}
	return result[:length]
}

func paddedScalar(value *big.Int, size int) []byte {
	raw := value.Bytes()
	out := make([]byte, size)
	copy(out[size-len(raw):], raw)
	return out
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	for _, candidate := range addresses {
		ip := candidate.IP
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			continue
		}
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		err = dialErr
	}
	if err == nil {
		err = errors.New("push endpoint resolved only to non-public addresses")
	}
	return nil, err
}
