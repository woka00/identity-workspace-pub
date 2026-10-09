package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

type LogSource interface {
	Lines(limit int) []string
}

type LogBuffer struct {
	mu       sync.Mutex
	capacity int
	lines    []string
	pending  string
}

var (
	logSecretPattern = regexp.MustCompile(`(?i)\b(access_token|refresh_token|oauth_token|oauth_verifier|client_secret|consumer_secret|password|token|secret|code|state)=([^&\s]+)`)
	logBearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[^\s]+`)
)

func NewLogBuffer(capacity int) *LogBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &LogBuffer{capacity: capacity, lines: make([]string, 0, capacity)}
}

func redactLogLine(value string) string {
	value = logSecretPattern.ReplaceAllString(value, "$1=[REDACTED]")
	return logBearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
}

func (buffer *LogBuffer) appendLine(line string) {
	line = strings.TrimSuffix(line, "\r")
	if line == "" {
		return
	}
	line = redactLogLine(line)
	if len(line) > 8_000 {
		line = line[:8_000] + "…"
	}
	if len(buffer.lines) == buffer.capacity {
		copy(buffer.lines, buffer.lines[1:])
		buffer.lines[len(buffer.lines)-1] = line
		return
	}
	buffer.lines = append(buffer.lines, line)
}

func (buffer *LogBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	buffer.pending += string(value)
	for {
		newline := strings.IndexByte(buffer.pending, '\n')
		if newline < 0 {
			break
		}
		buffer.appendLine(buffer.pending[:newline])
		buffer.pending = buffer.pending[newline+1:]
	}
	return len(value), nil
}

func (buffer *LogBuffer) Lines(limit int) []string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if limit <= 0 || limit > len(buffer.lines) {
		limit = len(buffer.lines)
	}
	start := len(buffer.lines) - limit
	return append([]string(nil), buffer.lines[start:]...)
}

func parseAdminLogLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 200, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 1000 {
		return 0, fmt.Errorf("количество строк должно быть от 1 до 1000: %w", domain.ErrInvalidInput)
	}
	return limit, nil
}

func (s *Server) adminLogs(w http.ResponseWriter, r *http.Request) {
	if err := application.RequireAdmin(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	limit, err := parseAdminLogLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, err)
		return
	}
	lines := []string{}
	if s.logs != nil {
		lines = s.logs.Lines(limit)
	}
	writeJSON(w, http.StatusOK, struct {
		Lines []string `json:"lines"`
	}{Lines: lines})
}
