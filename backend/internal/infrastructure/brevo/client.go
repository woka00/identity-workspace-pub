package brevo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

type Client struct {
	apiKey      string
	senderEmail string
	senderName  string
	httpClient  *http.Client
}

func New(apiKey, senderEmail, senderName string) (*Client, error) {
	address, err := mail.ParseAddress(senderEmail)
	if strings.TrimSpace(apiKey) == "" || strings.ContainsAny(apiKey, "\r\n") || err != nil || address.Address != senderEmail || strings.ContainsAny(senderName, "\r\n") || len(senderName) > 100 {
		return nil, errors.New("Brevo requires a valid API key and sender email/name")
	}
	return &Client{apiKey: apiKey, senderEmail: senderEmail, senderName: senderName, httpClient: &http.Client{
		Timeout: 10 * time.Second,
		// Never forward the API key to a redirect target.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) SendVerification(ctx context.Context, email, verificationURL string) error {
	body, err := json.Marshal(map[string]any{
		"sender":      map[string]string{"email": c.senderEmail, "name": c.senderName},
		"to":          []map[string]any{{"email": email, "contactPixelTrackingConsent": false}},
		"subject":     "Подтвердите почту — identity workspace",
		"textContent": "Вы запросили регистрацию в identity workspace.\n\nОткройте ссылку, чтобы подтвердить почту и выбрать логин и пароль:\n" + verificationURL + "\n\nСсылка действует 30 минут и используется один раз. Если вы запросили новое письмо, используйте последнюю ссылку.\n\nЕсли вы не запрашивали регистрацию, просто проигнорируйте это письмо. Не пересылайте ссылку другим людям.",
	})
	if err != nil {
		return errors.New("Brevo request encoding failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.brevo.com/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return errors.New("Brevo request creation failed")
	}
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return errors.New("Brevo delivery request failed")
	}
	defer response.Body.Close()
	// Provider responses can echo recipients and request content. Never log or
	// return that body (nor the API key) to callers.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("Brevo delivery rejected (HTTP %d)", response.StatusCode)
	}
	return nil
}
