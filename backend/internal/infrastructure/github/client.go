package github

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"avatar-id/internal/domain"
)

const (
	defaultBaseURL   = "https://github.com"
	maxResponseBytes = 4_000_000
)

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

func NewClient() *Client { return &Client{HTTPClient: defaultHTTPClient()} }

var (
	tableCellTagPattern      = regexp.MustCompile(`(?i)<td\b[^>]*>`)
	tooltipPattern           = regexp.MustCompile(`(?is)<tool-tip\b([^>]*)>([^<]*)</tool-tip>`)
	attributePattern         = regexp.MustCompile(`([A-Za-z_:][-A-Za-z0-9_:.]*)="([^"]*)"`)
	contributionCountPattern = regexp.MustCompile(`^([0-9][0-9,]*) contributions?\b`)
)

func (c Client) PublicContributions(ctx context.Context, username string) ([]domain.GitHubActivityDay, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("github username is required")
	}
	endpoint := c.baseURL() + "/users/" + url.PathEscape(username) + "/contributions"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	request.Header.Set("User-Agent", "identity-workspace/1.0")
	response, err := c.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("github contribution calendar: %w", err)
	}
	body, readErr := readLimited(response.Body, maxResponseBytes)
	response.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("github contribution calendar: %w", readErr)
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("github user %q: %w", username, domain.ErrNotFound)
	}
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("github contribution calendar rate limit reached")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("github contribution calendar: unexpected HTTP status %d", response.StatusCode)
	}
	result, err := parseContributionCalendar(body)
	if err != nil {
		return nil, fmt.Errorf("github contribution calendar response: %w", err)
	}
	return result, nil
}

func (c Client) baseURL() string {
	if value := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/"); value != "" {
		return value
	}
	return defaultBaseURL
}

func parseContributionCalendar(body []byte) ([]domain.GitHubActivityDay, error) {
	countsByTooltip := make(map[string]int)
	for _, match := range tooltipPattern.FindAllSubmatch(body, -1) {
		attributes := parseHTMLAttributes(match[1])
		forID := attributes["for"]
		if forID == "" {
			continue
		}
		text := strings.TrimSpace(html.UnescapeString(string(match[2])))
		if strings.HasPrefix(text, "No contributions") {
			countsByTooltip[forID] = 0
			continue
		}
		countMatch := contributionCountPattern.FindStringSubmatch(text)
		if len(countMatch) != 2 {
			continue
		}
		count, err := strconv.Atoi(strings.ReplaceAll(countMatch[1], ",", ""))
		if err == nil {
			countsByTooltip[forID] = count
		}
	}

	result := make([]domain.GitHubActivityDay, 0, 371)
	for _, tag := range tableCellTagPattern.FindAll(body, -1) {
		attributes := parseHTMLAttributes(tag)
		if !containsHTMLClass(attributes["class"], "ContributionCalendar-day") {
			continue
		}
		date := attributes["data-date"]
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		count, found := countsByTooltip[attributes["id"]]
		if !found {
			continue
		}
		level, err := strconv.Atoi(attributes["data-level"])
		if err != nil || level < 0 || level > 4 {
			continue
		}
		result = append(result, domain.GitHubActivityDay{Date: date, Count: count, Level: level})
	}
	if len(result) < 7 {
		return nil, errors.New("contribution calendar is unavailable")
	}
	return result, nil
}

func containsHTMLClass(value, expected string) bool {
	for _, item := range strings.Fields(value) {
		if item == expected {
			return true
		}
	}
	return false
}

func parseHTMLAttributes(tag []byte) map[string]string {
	result := make(map[string]string)
	for _, match := range attributePattern.FindAllSubmatch(tag, -1) {
		result[strings.ToLower(string(match[1]))] = html.UnescapeString(string(match[2]))
	}
	return result
}

func (c Client) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient()
}

func defaultHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 15 * time.Second
	transport.IdleConnTimeout = 60 * time.Second
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	limited := io.LimitReader(reader, limit+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("response is too large")
	}
	return body, nil
}
