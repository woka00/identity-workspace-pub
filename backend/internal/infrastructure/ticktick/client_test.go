package ticktick

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"identity-workspace/internal/domain"
)

func TestAuthorizeURL(t *testing.T) {
	client := Client{ClientID: "client-id", AuthorizationURL: "https://example.test/oauth"}
	parsed, err := url.Parse(client.AuthorizeURL("https://app.test/api/integrations/ticktick/callback", "state-value"))
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Host != "example.test" || query.Get("client_id") != "client-id" || query.Get("state") != "state-value" ||
		query.Get("response_type") != "code" || query.Get("scope") != oauthScope {
		t.Fatalf("unexpected authorize URL: %s", parsed.String())
	}
}

func TestAccessTokenUsesBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "client" || password != "secret" {
			t.Fatalf("unexpected basic auth: %q / %q", username, password)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("code") != "code" || r.Form.Get("redirect_uri") != "https://app.test/callback" {
			t.Fatalf("unexpected token form: %#v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "access"})
	}))
	defer server.Close()

	client := Client{ClientID: "client", ClientSecret: "secret", TokenURL: server.URL, HTTPClient: server.Client()}
	token, err := client.AccessToken(context.Background(), "code", "https://app.test/callback")
	if err != nil {
		t.Fatal(err)
	}
	if token != "access" {
		t.Fatalf("unexpected token: %q", token)
	}
}

func TestCreateAndCompleteTask(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("missing bearer token")
		}
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/task":
			var payload taskPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.ProjectID != "project" || payload.Title != "Название" || payload.Content != "Описание" || payload.DueDate != "2026-08-07T11:30:00+0000" || payload.TimeZone != "Europe/Moscow" || payload.Priority != 3 || payload.IsAllDay {
				t.Fatalf("unexpected task payload: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "remote-task", "projectId": "project"})
		case "/project/project/task/remote-task/complete":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := Client{APIBaseURL: server.URL, HTTPClient: server.Client(), TimeZone: "Europe/Moscow"}
	taskID, err := client.CreateTask(context.Background(), "access", "project", domain.Task{
		Title: "Название", Description: "Описание", DueDate: "2026-08-07", DueTime: "14:30", Priority: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CompleteTask(context.Background(), "access", "project", taskID); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(requests, " | ")
	if joined != "POST /task | POST /project/project/task/remote-task/complete" {
		t.Fatalf("unexpected requests: %s", joined)
	}
}

func TestPayloadForAllDayTaskUsesConfiguredTimeZone(t *testing.T) {
	client := Client{TimeZone: "Europe/Moscow"}
	payload, err := client.payloadForTask(domain.Task{Title: "На сегодня", DueDate: "2026-08-11"}, "project", false)
	if err != nil {
		t.Fatal(err)
	}
	if !payload.IsAllDay || payload.TimeZone != "Europe/Moscow" || payload.DueDate != "2026-08-10T21:00:00+0000" {
		t.Fatalf("unexpected all-day payload: %#v", payload)
	}
}

func TestEnsureProjectReusesLegacyProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/project" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]project{{ID: "p1", Name: "AVATAR.ID"}})
	}))
	defer server.Close()
	client := Client{APIBaseURL: server.URL, HTTPClient: server.Client()}
	id, name, err := client.EnsureProject(context.Background(), "access")
	if err != nil {
		t.Fatal(err)
	}
	if id != "p1" || name != "AVATAR.ID" {
		t.Fatalf("unexpected project: %q %q", id, name)
	}
}

func TestEnsureProjectCreatesProjectWithCurrentName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/project":
			_ = json.NewEncoder(w).Encode([]project{})
		case r.Method == http.MethodPost && r.URL.Path == "/project":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "identity workspace" {
				t.Fatalf("unexpected project name: %q", payload["name"])
			}
			_ = json.NewEncoder(w).Encode(project{ID: "p2", Name: payload["name"]})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := Client{APIBaseURL: server.URL, HTTPClient: server.Client()}
	id, name, err := client.EnsureProject(context.Background(), "access")
	if err != nil {
		t.Fatal(err)
	}
	if id != "p2" || name != "identity workspace" {
		t.Fatalf("unexpected project: %q %q", id, name)
	}
}

func TestListTasksUsesGlobalFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/task/filter" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("missing bearer token")
		}
		var payload struct {
			Status []int `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Status) != 1 || payload.Status[0] != 0 {
			t.Fatalf("unexpected filter: %#v", payload)
		}
		_ = json.NewEncoder(w).Encode([]remoteTask{
			{ID: "t1", ProjectID: "p1", Title: "Работа", Content: "Текст", DueDate: "2026-08-10T21:00:00.000+0000", TimeZone: "Europe/Moscow", IsAllDay: true, Priority: 5, Status: 0},
			{ID: "t2", ProjectID: "p2", Title: "Встреча", DueDate: "2026-08-11T11:30:00.000+0000", TimeZone: "Europe/Moscow", Status: 0},
			{ID: "t3", ProjectID: "p2", Title: "Дом", Desc: "Описание списка", Status: 0},
		})
	}))
	defer server.Close()

	client := Client{APIBaseURL: server.URL, HTTPClient: server.Client(), TimeZone: "Europe/Moscow"}
	tasks, err := client.ListTasks(context.Background(), "access")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 || tasks[0].DueDate != "2026-08-11" || tasks[0].DueTime != "" || tasks[0].Priority != 3 || !tasks[0].IsMilestone || tasks[1].DueDate != "2026-08-11" || tasks[1].DueTime != "14:30" || tasks[2].Description != "Описание списка" {
		t.Fatalf("unexpected tasks: %#v", tasks)
	}
}
