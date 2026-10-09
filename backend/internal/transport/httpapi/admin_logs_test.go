package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

func TestLogBufferKeepsRecentRedactedLines(t *testing.T) {
	buffer := NewLogBuffer(2)
	_, _ = buffer.Write([]byte("first\nsecond token=visible\nthird Authorization: Bearer visible\n"))
	lines := buffer.Lines(10)
	if len(lines) != 2 || !strings.Contains(lines[0], "token=[REDACTED]") || !strings.Contains(lines[1], "Bearer [REDACTED]") {
		t.Fatalf("unexpected buffered logs: %#v", lines)
	}
	if strings.Contains(strings.Join(lines, "\n"), "visible") {
		t.Fatalf("secret value was retained: %#v", lines)
	}
}

func TestAdminLogsRequiresAdministrator(t *testing.T) {
	buffer := NewLogBuffer(10)
	_, _ = buffer.Write([]byte("ready\n"))
	server := &Server{logs: buffer}

	nonAdminContext := application.WithUser(context.Background(), domain.User{ID: 1})
	nonAdminRequest := httptest.NewRequest(http.MethodGet, "/api/admin/logs", nil).WithContext(nonAdminContext)
	nonAdminResponse := httptest.NewRecorder()
	server.adminLogs(nonAdminResponse, nonAdminRequest)
	if nonAdminResponse.Code != http.StatusForbidden {
		t.Fatalf("non-admin status=%d, want %d", nonAdminResponse.Code, http.StatusForbidden)
	}

	adminContext := application.WithUser(context.Background(), domain.User{ID: 1, IsAdmin: true})
	adminRequest := httptest.NewRequest(http.MethodGet, "/api/admin/logs?limit=1", nil).WithContext(adminContext)
	adminResponse := httptest.NewRecorder()
	server.adminLogs(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusOK || !strings.Contains(adminResponse.Body.String(), "ready") {
		t.Fatalf("admin response status=%d body=%q", adminResponse.Code, adminResponse.Body.String())
	}
}
