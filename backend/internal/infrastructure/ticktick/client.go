package ticktick

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"identity-workspace/internal/domain"
)

const (
	defaultAuthorizationURL        = "https://ticktick.com/oauth/authorize"
	defaultTokenURL                = "https://ticktick.com/oauth/token"
	defaultAPIBaseURL              = "https://api.ticktick.com/open/v1"
	oauthScope                     = "tasks:read tasks:write"
	integrationProjectName         = "identity workspace"
	previousIntegrationProjectName = "identity-workspace"
	legacyIntegrationProjectName   = "AVATAR.ID"
)

type Client struct {
	ClientID         string
	ClientSecret     string
	TimeZone         string
	HTTPClient       *http.Client
	AuthorizationURL string
	TokenURL         string
	APIBaseURL       string
}

type project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

type remoteTask struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Desc      string `json:"desc"`
	DueDate   string `json:"dueDate"`
	TimeZone  string `json:"timeZone"`
	IsAllDay  bool   `json:"isAllDay"`
	Priority  int    `json:"priority"`
	Status    int    `json:"status"`
}

type taskPayload struct {
	ID        string `json:"id,omitempty"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Content   string `json:"content,omitempty"`
	DueDate   string `json:"dueDate,omitempty"`
	TimeZone  string `json:"timeZone,omitempty"`
	IsAllDay  bool   `json:"isAllDay"`
	Priority  int    `json:"priority"`
	Status    *int   `json:"status,omitempty"`
}

func (c Client) Configured() bool {
	return strings.TrimSpace(c.ClientID) != "" && strings.TrimSpace(c.ClientSecret) != ""
}

func (c Client) AuthorizeURL(callbackURL, state string) string {
	endpoint := c.authorizationURL()
	values := url.Values{}
	values.Set("client_id", strings.TrimSpace(c.ClientID))
	values.Set("scope", oauthScope)
	values.Set("state", state)
	values.Set("redirect_uri", callbackURL)
	values.Set("response_type", "code")
	return endpoint + "?" + values.Encode()
}

func (c Client) AccessToken(ctx context.Context, code, callbackURL string) (string, error) {
	if !c.Configured() {
		return "", errors.New("ticktick integration is not configured")
	}
	values := url.Values{}
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("scope", oauthScope)
	values.Set("redirect_uri", callbackURL)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL(), strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	request.SetBasicAuth(strings.TrimSpace(c.ClientID), strings.TrimSpace(c.ClientSecret))
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "identity-workspace/1.0")

	response, err := c.client().Do(request)
	if err != nil {
		return "", fmt.Errorf("ticktick access token: %w", err)
	}
	defer response.Body.Close()
	body, err := readLimitedBody(response.Body, 256_000)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("ticktick access token: %s", tickTickErrorText(response.Status, body))
	}
	var payload tokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("ticktick access token response: %w", err)
	}
	payload.AccessToken = strings.TrimSpace(payload.AccessToken)
	if payload.AccessToken == "" {
		return "", errors.New("ticktick access token response is incomplete")
	}
	return payload.AccessToken, nil
}

func (c Client) EnsureProject(ctx context.Context, accessToken string) (string, string, error) {
	var projects []project
	if err := c.apiJSON(ctx, http.MethodGet, "/project", accessToken, nil, &projects); err != nil {
		return "", "", err
	}
	var legacyProject *project
	for _, item := range projects {
		name := strings.TrimSpace(item.Name)
		if strings.EqualFold(name, integrationProjectName) && strings.TrimSpace(item.ID) != "" {
			return item.ID, item.Name, nil
		}
		if legacyProject == nil && (strings.EqualFold(name, previousIntegrationProjectName) || strings.EqualFold(name, legacyIntegrationProjectName)) && strings.TrimSpace(item.ID) != "" {
			itemCopy := item
			legacyProject = &itemCopy
		}
	}
	// Existing installations keep using their original TickTick project. This
	// prevents a reconnect after the rebrand from creating duplicate task lists.
	if legacyProject != nil {
		return legacyProject.ID, legacyProject.Name, nil
	}
	var created project
	if err := c.apiJSON(ctx, http.MethodPost, "/project", accessToken, map[string]string{
		"name":  integrationProjectName,
		"color": "#171717",
	}, &created); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", "", errors.New("ticktick project response is incomplete")
	}
	if strings.TrimSpace(created.Name) == "" {
		created.Name = integrationProjectName
	}
	return created.ID, created.Name, nil
}

func (c Client) ListTasks(ctx context.Context, accessToken string) ([]domain.TickTickRemoteTask, error) {
	// Официальный filter endpoint возвращает задачи сразу по всем спискам,
	// поэтому один открытый клиент делает один запрос на каждую синхронизацию.
	var tasks []remoteTask
	if err := c.apiJSON(ctx, http.MethodPost, "/task/filter", accessToken, map[string]any{
		"status": []int{0},
	}, &tasks); err != nil {
		return nil, err
	}
	out := make([]domain.TickTickRemoteTask, 0, len(tasks))
	for _, task := range tasks {
		description := strings.TrimSpace(task.Content)
		if description == "" {
			description = strings.TrimSpace(task.Desc)
		}
		dueDate, dueTime := c.tickTickDateTime(task.DueDate, task.TimeZone, task.IsAllDay)
		out = append(out, domain.TickTickRemoteTask{
			ID:          task.ID,
			ProjectID:   task.ProjectID,
			Title:       task.Title,
			Description: description,
			DueDate:     dueDate,
			DueTime:     dueTime,
			Priority:    tickTickPriorityFromRemote(task.Priority),
			Status:      "todo",
			IsMilestone: task.Priority == 5,
		})
	}
	return out, nil
}

func (c Client) tickTickDateTime(value, remoteTimeZone string, isAllDay bool) (string, string) {
	value = strings.TrimSpace(value)
	if len(value) < len("2006-01-02") {
		return "", ""
	}
	date := value[:len("2006-01-02")]
	if parsed, err := time.Parse("2006-01-02", date); err != nil || parsed.Format("2006-01-02") != date {
		return "", ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05-0700", "2006-01-02T15:04:05.000-0700"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			localized := parsed.In(c.location(remoteTimeZone))
			if isAllDay {
				return localized.Format("2006-01-02"), ""
			}
			return localized.Format("2006-01-02"), localized.Format("15:04")
		}
	}
	return date, ""
}

func (c Client) location(remoteTimeZone string) *time.Location {
	for _, name := range []string{strings.TrimSpace(remoteTimeZone), strings.TrimSpace(c.TimeZone), "UTC"} {
		if name == "" {
			continue
		}
		if location, err := time.LoadLocation(name); err == nil {
			return location
		}
	}
	return time.UTC
}

func (c Client) timeZoneName() string {
	name := strings.TrimSpace(c.TimeZone)
	if name == "" {
		return "UTC"
	}
	return name
}

func tickTickPriorityFromRemote(value int) int {
	switch {
	case value >= 5:
		return 3
	case value >= 3:
		return 2
	case value >= 1:
		return 1
	default:
		return 0
	}
}

func tickTickPriorityToRemote(value int) int {
	switch value {
	case 3:
		return 5
	case 2:
		return 3
	case 1:
		return 1
	default:
		return 0
	}
}

func (c Client) CreateTask(ctx context.Context, accessToken, projectID string, task domain.Task) (string, error) {
	payload, err := c.payloadForTask(task, projectID, false)
	if err != nil {
		return "", err
	}
	var created remoteTask
	if err := c.apiJSON(ctx, http.MethodPost, "/task", accessToken, payload, &created); err != nil {
		return "", err
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", errors.New("ticktick create task response is incomplete")
	}
	return created.ID, nil
}

func (c Client) UpdateTask(ctx context.Context, accessToken, projectID, taskID string, task domain.Task) error {
	payload, err := c.payloadForTask(task, projectID, true)
	if err != nil {
		return err
	}
	payload.ID = taskID
	return c.apiJSON(ctx, http.MethodPost, "/task/"+url.PathEscape(taskID), accessToken, payload, nil)
}

func (c Client) CompleteTask(ctx context.Context, accessToken, projectID, taskID string) error {
	path := "/project/" + url.PathEscape(projectID) + "/task/" + url.PathEscape(taskID) + "/complete"
	return c.apiJSON(ctx, http.MethodPost, path, accessToken, nil, nil)
}

func (c Client) DeleteTask(ctx context.Context, accessToken, projectID, taskID string) error {
	path := "/project/" + url.PathEscape(projectID) + "/task/" + url.PathEscape(taskID)
	return c.apiJSON(ctx, http.MethodDelete, path, accessToken, nil, nil)
}

func (c Client) payloadForTask(task domain.Task, projectID string, includeStatus bool) (taskPayload, error) {
	payload := taskPayload{
		ProjectID: projectID,
		Title:     task.Title,
		Content:   task.Description,
		IsAllDay:  task.DueTime == "",
		Priority:  tickTickPriorityToRemote(task.Priority),
	}
	if task.DueDate != "" {
		value := task.DueDate
		layout := "2006-01-02"
		if task.DueTime != "" {
			value += "T" + task.DueTime
			layout = "2006-01-02T15:04"
		}
		parsed, err := time.ParseInLocation(layout, value, c.location(""))
		if err != nil {
			return taskPayload{}, fmt.Errorf("ticktick task date: %w", err)
		}
		payload.DueDate = parsed.UTC().Format("2006-01-02T15:04:05-0700")
		payload.TimeZone = c.timeZoneName()
	}
	// Для завершения используется отдельная команда /complete. При возврате
	// задачи в работу явно отправляем status=0 через update endpoint.
	if includeStatus && task.Status != "done" {
		status := 0
		payload.Status = &status
	}
	return payload, nil
}

func (c Client) apiJSON(ctx context.Context, method, path, accessToken string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.apiBaseURL(), "/")+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	request.Header.Set("User-Agent", "identity-workspace/1.0")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client().Do(request)
	if err != nil {
		return fmt.Errorf("ticktick API %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	responseBody, err := readLimitedBody(response.Body, 1_000_000)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("ticktick API %s %s: %s", method, path, tickTickErrorText(response.Status, responseBody))
	}
	if output == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("ticktick API %s %s response: %w", method, path, err)
	}
	return nil
}

func (c Client) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
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

func (c Client) authorizationURL() string {
	if value := strings.TrimSpace(c.AuthorizationURL); value != "" {
		return value
	}
	return defaultAuthorizationURL
}

func (c Client) tokenURL() string {
	if value := strings.TrimSpace(c.TokenURL); value != "" {
		return value
	}
	return defaultTokenURL
}

func (c Client) apiBaseURL() string {
	if value := strings.TrimSpace(c.APIBaseURL); value != "" {
		return value
	}
	return defaultAPIBaseURL
}

func tickTickErrorText(status string, body []byte) string {
	var payload struct {
		Error       string `json:"error"`
		ErrorCode   string `json:"errorCode"`
		Description string `json:"error_description"`
		Message     string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		for _, value := range []string{payload.Description, payload.Message, payload.Error, payload.ErrorCode} {
			if strings.TrimSpace(value) != "" {
				return status + ": " + strings.TrimSpace(value)
			}
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 300 {
		text = text[:300]
	}
	if text == "" {
		return status
	}
	return status + ": " + text
}

func readLimitedBody(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("ticktick response is too large")
	}
	return body, nil
}
