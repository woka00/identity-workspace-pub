package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"identity-workspace/internal/domain"
)

const tickTickErrorLimit = 500

type TickTickGateway interface {
	Configured() bool
	AuthorizeURL(callbackURL, state string) string
	AccessToken(context.Context, string, string) (string, error)
	EnsureProject(context.Context, string) (string, string, error)
	ListTasks(context.Context, string) ([]domain.TickTickRemoteTask, error)
	CreateTask(context.Context, string, string, domain.Task) (string, error)
	UpdateTask(context.Context, string, string, string, domain.Task) error
	CompleteTask(context.Context, string, string, string) error
	DeleteTask(context.Context, string, string, string) error
}

func (s *Service) TickTickStatus(ctx context.Context) (domain.TickTickStatus, error) {
	status := domain.TickTickStatus{Configured: s.tickTick != nil && s.tickTick.Configured()}
	connection, err := s.repo.TickTickConnection(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return status, nil
	}
	if err != nil {
		return domain.TickTickStatus{}, err
	}
	pending, failed, err := s.repo.TickTickSyncCounts(ctx)
	if err != nil {
		return domain.TickTickStatus{}, err
	}
	status.Connected = true
	status.ConnectedAt = connection.ConnectedAt
	status.ProjectName = connection.ProjectName
	status.PendingTasks = pending
	status.FailedTasks = failed
	return status, nil
}

func (s *Service) BeginTickTickConnection(ctx context.Context, callbackURL, returnTo string) (string, error) {
	if s.tickTick == nil || !s.tickTick.Configured() {
		return "", fmt.Errorf("TickTick is not configured: %w", domain.ErrConflict)
	}
	state, err := randomOAuthState()
	if err != nil {
		return "", err
	}
	pending := domain.TickTickOAuthState{
		State:       state,
		CallbackURL: strings.TrimSpace(callbackURL),
		ReturnTo:    strings.TrimSpace(returnTo),
	}
	if err := s.repo.SaveTickTickOAuthState(ctx, pending); err != nil {
		return "", err
	}
	return s.tickTick.AuthorizeURL(pending.CallbackURL, state), nil
}

func (s *Service) CompleteTickTickConnection(ctx context.Context, state, code string) (string, error) {
	if s.tickTick == nil || !s.tickTick.Configured() {
		return "", fmt.Errorf("TickTick is not configured: %w", domain.ErrConflict)
	}
	state = strings.TrimSpace(state)
	code = strings.TrimSpace(code)
	if state == "" || code == "" {
		return "", fmt.Errorf("TickTick authorization was denied: %w", domain.ErrConflict)
	}
	pending, err := s.repo.ConsumeTickTickOAuthState(ctx, state)
	if err != nil {
		return "", err
	}
	accessToken, err := s.tickTick.AccessToken(ctx, code, pending.CallbackURL)
	if err != nil {
		return pending.ReturnTo, err
	}
	projectID, projectName, err := s.tickTick.EnsureProject(ctx, accessToken)
	if err != nil {
		return pending.ReturnTo, err
	}
	if err := s.repo.SaveTickTickConnection(ctx, pending.UserID, accessToken, projectID, projectName); err != nil {
		return pending.ReturnTo, err
	}
	return pending.ReturnTo, nil
}

func (s *Service) DisconnectTickTick(ctx context.Context) error {
	unlock := s.lockTickTick(ctx)
	defer unlock()
	return s.repo.DeleteTickTickConnection(ctx)
}

func (s *Service) SyncTickTick(ctx context.Context) (domain.TickTickSyncResult, error) {
	unlock := s.lockTickTick(ctx)
	defer unlock()
	if s.tickTick == nil || !s.tickTick.Configured() {
		return domain.TickTickSyncResult{}, fmt.Errorf("TickTick is not configured: %w", domain.ErrConflict)
	}
	connection, err := s.repo.TickTickConnection(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.TickTickSyncResult{}, fmt.Errorf("TickTick account is not connected: %w", domain.ErrConflict)
	}
	if err != nil {
		return domain.TickTickSyncResult{}, err
	}

	result := domain.TickTickSyncResult{}
	pending, err := s.repo.TickTickPendingTasks(ctx)
	if err != nil {
		return domain.TickTickSyncResult{}, err
	}
	// Сначала отправляем локальные изменения. Тогда последующий снимок TickTick
	// уже отражает актуальное состояние и не затирает только что сохранённые данные.
	for _, task := range pending {
		updated, syncErr := s.syncTickTickTask(ctx, task, false)
		if syncErr != nil || updated.TickTickSyncStatus == "error" {
			result.Failed++
			continue
		}
		result.Synced++
	}

	remoteTasks, err := s.tickTick.ListTasks(ctx, connection.AccessToken)
	if err != nil {
		return result, err
	}
	pull, err := s.repo.ApplyTickTickSnapshot(ctx, normalizeTickTickRemoteTasks(remoteTasks))
	if err != nil {
		return result, err
	}
	result.Imported = pull.Imported
	result.Updated = pull.Updated
	result.Completed = pull.Completed
	return result, nil
}

func normalizeTickTickRemoteTasks(tasks []domain.TickTickRemoteTask) []domain.TickTickRemoteTask {
	out := make([]domain.TickTickRemoteTask, 0, len(tasks))
	seen := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		task.ID = strings.TrimSpace(task.ID)
		task.ProjectID = strings.TrimSpace(task.ProjectID)
		if task.ID == "" || task.ProjectID == "" {
			continue
		}
		if _, exists := seen[task.ID]; exists {
			continue
		}
		seen[task.ID] = struct{}{}
		task.Title = truncateTickTickText(strings.TrimSpace(task.Title), 160)
		if task.Title == "" {
			task.Title = "Задача TickTick"
		}
		task.Description = truncateTickTickText(strings.TrimSpace(task.Description), 2000)
		task.DueDate = strings.TrimSpace(task.DueDate)
		task.Status = "todo"
		out = append(out, task)
	}
	return out
}

func truncateTickTickText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (s *Service) syncTickTickTaskBestEffort(ctx context.Context, task domain.Task, createLink bool) domain.Task {
	unlock := s.lockTickTick(ctx)
	defer unlock()
	updated, err := s.syncTickTickTask(ctx, task, createLink)
	if err == nil {
		return updated
	}
	task.TickTickSyncStatus = "error"
	task.TickTickSyncError = shortTickTickError(err)
	return task
}

func (s *Service) syncTickTickTask(ctx context.Context, task domain.Task, createLink bool) (domain.Task, error) {
	if s.tickTick == nil || !s.tickTick.Configured() {
		return task, nil
	}
	connection, err := s.repo.TickTickConnection(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return task, nil
	}
	if err != nil {
		return task, err
	}

	link, err := s.repo.TickTickTaskLink(ctx, task.ID)
	if errors.Is(err, domain.ErrNotFound) {
		if !createLink {
			return task, nil
		}
		link = domain.TickTickTaskLink{TaskID: task.ID, ProjectID: connection.ProjectID, SyncStatus: "pending"}
		if err := s.repo.SaveTickTickTaskLink(ctx, link); err != nil {
			return task, err
		}
	} else if err != nil {
		return task, err
	}
	if strings.TrimSpace(link.ProjectID) == "" {
		link.ProjectID = connection.ProjectID
	}
	link.SyncStatus = "pending"
	link.LastError = ""
	if err := s.repo.SaveTickTickTaskLink(ctx, link); err != nil {
		return task, err
	}

	if strings.TrimSpace(link.TickTickTaskID) == "" {
		link.TickTickTaskID, err = s.tickTick.CreateTask(ctx, connection.AccessToken, link.ProjectID, task)
		if err == nil {
			// Сохраняем remote ID сразу: если последующая команда завершения временно
			// не сработает, повторная синхронизация обновит ту же задачу без дубля.
			if saveErr := s.repo.SaveTickTickTaskLink(ctx, link); saveErr != nil {
				return task, saveErr
			}
		}
	} else {
		err = s.tickTick.UpdateTask(ctx, connection.AccessToken, link.ProjectID, link.TickTickTaskID, task)
	}
	if err == nil && task.Status == "done" {
		err = s.tickTick.CompleteTask(ctx, connection.AccessToken, link.ProjectID, link.TickTickTaskID)
	}
	if err != nil {
		link.SyncStatus = "error"
		link.LastError = shortTickTickError(err)
		if saveErr := s.repo.SaveTickTickTaskLink(ctx, link); saveErr != nil {
			return task, saveErr
		}
		task.TickTickSyncStatus = link.SyncStatus
		task.TickTickSyncError = link.LastError
		return task, nil
	}

	link.SyncStatus = "synced"
	link.LastError = ""
	if err := s.repo.SaveTickTickTaskLink(ctx, link); err != nil {
		return task, err
	}
	task.TickTickSyncStatus = link.SyncStatus
	task.TickTickSyncError = ""
	return task, nil
}

func (s *Service) lockTickTick(ctx context.Context) func() {
	userID, err := domain.UserID(ctx)
	if err != nil {
		// Unit tests and internal callers without HTTP middleware share one lock.
		userID = 0
	}
	s.tickTickLocksMu.Lock()
	lock := s.tickTickLocks[userID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.tickTickLocks[userID] = lock
	}
	s.tickTickLocksMu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func randomOAuthState() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate TickTick OAuth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func shortTickTickError(err error) string {
	text := strings.TrimSpace(err.Error())
	if len(text) > tickTickErrorLimit {
		text = text[:tickTickErrorLimit]
	}
	return text
}
