package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"identity-workspace/internal/domain"
)

type PushGateway interface {
	Configured() bool
	PublicKey() string
	Send(context.Context, domain.PushSubscription, []byte) (int, error)
}

func (s *Service) TaskCategories(ctx context.Context) ([]domain.TaskCategory, error) {
	return s.repo.TaskCategories(ctx)
}

func (s *Service) CreateTaskCategory(ctx context.Context, name string) (domain.TaskCategory, error) {
	normalized, err := NormalizeCategory(name)
	if err != nil {
		return domain.TaskCategory{}, err
	}
	return s.repo.CreateTaskCategory(ctx, normalized)
}

func (s *Service) DeleteTaskCategory(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid task category id")
	}
	return s.repo.DeleteTaskCategory(ctx, id)
}

func (s *Service) CreateCustomTracker(ctx context.Context, input domain.CustomTrackerInput) (domain.CustomTracker, error) {
	input, err := NormalizeCustomTracker(input)
	if err != nil {
		return domain.CustomTracker{}, err
	}
	return s.repo.CreateCustomTracker(ctx, input)
}

func (s *Service) UpdateCustomTracker(ctx context.Context, id int64, input domain.CustomTrackerInput) (domain.CustomTracker, error) {
	if id <= 0 {
		return domain.CustomTracker{}, invalidf("invalid custom tracker id")
	}
	input, err := NormalizeCustomTracker(input)
	if err != nil {
		return domain.CustomTracker{}, err
	}
	return s.repo.UpdateCustomTracker(ctx, id, input)
}

func (s *Service) StepCustomTracker(ctx context.Context, id int64, date string, direction int) (domain.CustomTracker, error) {
	if id <= 0 || (direction != -1 && direction != 1) {
		return domain.CustomTracker{}, invalidf("invalid custom tracker step")
	}
	date, err := NormalizeDate(date, "tracker date")
	if err != nil {
		return domain.CustomTracker{}, err
	}
	return s.repo.StepCustomTracker(ctx, id, date, direction)
}

func (s *Service) DeleteCustomTracker(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid custom tracker id")
	}
	return s.repo.DeleteCustomTracker(ctx, id)
}

type trackerReminderRepository interface {
	TrackerReminders(context.Context) ([]domain.TrackerReminder, error)
	UpsertTrackerReminder(context.Context, domain.TrackerReminderInput) (domain.TrackerReminder, error)
	DeleteTrackerReminder(context.Context, string) error
	ClaimDueTrackerReminders(context.Context, string, string, time.Time, int) ([]domain.TrackerReminderJob, error)
	CompleteTrackerReminder(context.Context, int64, string, string, bool) error
}

func (s *Service) trackerReminderRepository() (trackerReminderRepository, error) {
	repo, ok := s.repo.(trackerReminderRepository)
	if !ok {
		return nil, fmt.Errorf("tracker reminders repository is unavailable: %w", domain.ErrConflict)
	}
	return repo, nil
}

func normalizeTrackerReminder(input domain.TrackerReminderInput) (domain.TrackerReminderInput, int64, error) {
	input.TrackerKey = strings.TrimSpace(input.TrackerKey)
	input.Time = strings.TrimSpace(input.Time)
	if len(input.Time) != 5 || input.Time[2] != ':' {
		return domain.TrackerReminderInput{}, 0, invalidf("tracker reminder time must be HH:MM")
	}
	if parsed, err := time.Parse("15:04", input.Time); err != nil {
		return domain.TrackerReminderInput{}, 0, invalidf("tracker reminder time must be HH:MM")
	} else {
		input.Time = parsed.Format("15:04")
	}
	switch input.TrackerKey {
	case "calories", "water", "weight":
		return input, 0, nil
	}
	if !strings.HasPrefix(input.TrackerKey, "custom:") {
		return domain.TrackerReminderInput{}, 0, invalidf("invalid tracker reminder key")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(input.TrackerKey, "custom:"), 10, 64)
	if err != nil || id <= 0 {
		return domain.TrackerReminderInput{}, 0, invalidf("invalid tracker reminder key")
	}
	return input, id, nil
}

func (s *Service) TrackerReminders(ctx context.Context) ([]domain.TrackerReminder, error) {
	repo, err := s.trackerReminderRepository()
	if err != nil {
		return nil, err
	}
	return repo.TrackerReminders(ctx)
}

func (s *Service) SaveTrackerReminder(ctx context.Context, input domain.TrackerReminderInput) (domain.TrackerReminder, error) {
	normalized, customID, err := normalizeTrackerReminder(input)
	if err != nil {
		return domain.TrackerReminder{}, err
	}
	if customID > 0 {
		trackers, err := s.repo.CustomTrackers(ctx)
		if err != nil {
			return domain.TrackerReminder{}, err
		}
		found := false
		for _, tracker := range trackers {
			if tracker.ID == customID {
				found = true
				break
			}
		}
		if !found {
			return domain.TrackerReminder{}, fmt.Errorf("custom tracker: %w", domain.ErrNotFound)
		}
	}
	repo, err := s.trackerReminderRepository()
	if err != nil {
		return domain.TrackerReminder{}, err
	}
	return repo.UpsertTrackerReminder(ctx, normalized)
}

func (s *Service) DeleteTrackerReminder(ctx context.Context, trackerKey string) error {
	normalized, _, err := normalizeTrackerReminder(domain.TrackerReminderInput{TrackerKey: trackerKey, Time: "00:00"})
	if err != nil {
		return err
	}
	repo, err := s.trackerReminderRepository()
	if err != nil {
		return err
	}
	return repo.DeleteTrackerReminder(ctx, normalized.TrackerKey)
}

func (s *Service) NotificationConfig() domain.NotificationConfig {
	if s.push == nil || !s.push.Configured() {
		return domain.NotificationConfig{}
	}
	return domain.NotificationConfig{Configured: true, PublicKey: s.push.PublicKey()}
}

func (s *Service) SavePushSubscription(ctx context.Context, input domain.PushSubscriptionInput, userAgent string) error {
	if s.push == nil || !s.push.Configured() {
		return fmt.Errorf("push notifications are not configured: %w", domain.ErrConflict)
	}
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.P256DH = strings.TrimSpace(input.P256DH)
	input.Auth = strings.TrimSpace(input.Auth)
	if len(input.Endpoint) > 2048 || len(input.P256DH) > 256 || len(input.Auth) > 128 || input.Endpoint == "" || input.P256DH == "" || input.Auth == "" {
		return invalidf("invalid push subscription")
	}
	parsed, err := url.Parse(input.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return invalidf("invalid push endpoint")
	}
	publicKey, keyErr := base64.RawURLEncoding.DecodeString(input.P256DH)
	authSecret, authErr := base64.RawURLEncoding.DecodeString(input.Auth)
	if keyErr != nil || len(publicKey) != 65 || publicKey[0] != 4 || authErr != nil || len(authSecret) != 16 {
		return invalidf("invalid push subscription keys")
	}
	if runes := []rune(userAgent); len(runes) > 512 {
		userAgent = string(runes[:512])
	}
	return s.repo.SavePushSubscription(ctx, input, userAgent)
}

func (s *Service) DeletePushSubscription(ctx context.Context, endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || len(endpoint) > 2048 {
		return invalidf("invalid push endpoint")
	}
	return s.repo.DeletePushSubscription(ctx, endpoint)
}

func (s *Service) RunReminderWorker(ctx context.Context) {
	if s.push == nil || !s.push.Configured() {
		log.Print("reminders: Web Push is not configured")
		return
	}
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.deliverDueReminders(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("reminders: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) deliverDueReminders(ctx context.Context) error {
	if err := s.deliverDueTaskReminders(ctx); err != nil {
		return err
	}
	return s.deliverDueTrackerReminders(ctx)
}

func (s *Service) deliverDueTaskReminders(ctx context.Context) error {
	jobs, err := s.repo.ClaimDueReminders(ctx, time.Now().UTC(), 25)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		payload, err := json.Marshal(map[string]string{
			"title": "Напоминание · identity workspace",
			"body":  reminderBody(job),
			"url":   "/?view=tasks&task=" + fmt.Sprint(job.TaskID),
			"tag":   fmt.Sprintf("avatar-task-%d", job.TaskID),
		})
		if err != nil {
			_ = s.repo.CompleteReminder(ctx, job.TaskID, false)
			continue
		}
		sent := false
		for _, subscription := range job.Subscriptions {
			status, sendErr := s.push.Send(ctx, subscription, payload)
			if sendErr == nil && status >= 200 && status < 300 {
				sent = true
				continue
			}
			if status == 404 || status == 410 {
				_ = s.repo.DeletePushSubscriptionByID(ctx, subscription.ID)
			}
			if sendErr != nil {
				log.Printf("task reminder %d push %d: %v", job.TaskID, subscription.ID, sendErr)
			}
		}
		if err := s.repo.CompleteReminder(ctx, job.TaskID, sent); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) deliverDueTrackerReminders(ctx context.Context) error {
	repo, err := s.trackerReminderRepository()
	if err != nil {
		return err
	}
	localNow := s.now()
	jobs, err := repo.ClaimDueTrackerReminders(ctx, localNow.Format("2006-01-02"), localNow.Format("15:04"), time.Now().UTC(), 25)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		payload, err := json.Marshal(map[string]string{
			"title": "Трекер · identity workspace",
			"body":  "Пора обновить: " + strings.TrimSpace(job.Title),
			"url":   "/?view=card&tracker=" + url.QueryEscape(job.TrackerKey),
			"tag":   "avatar-tracker-" + strings.ReplaceAll(job.TrackerKey, ":", "-"),
		})
		if err != nil {
			_ = repo.CompleteTrackerReminder(ctx, job.UserID, job.TrackerKey, job.LocalDate, false)
			continue
		}
		sent := false
		for _, subscription := range job.Subscriptions {
			status, sendErr := s.push.Send(ctx, subscription, payload)
			if sendErr == nil && status >= 200 && status < 300 {
				sent = true
				continue
			}
			if status == 404 || status == 410 {
				_ = s.repo.DeletePushSubscriptionByID(ctx, subscription.ID)
			}
			if sendErr != nil {
				log.Printf("tracker reminder %s push %d: %v", job.TrackerKey, subscription.ID, sendErr)
			}
		}
		if err := repo.CompleteTrackerReminder(ctx, job.UserID, job.TrackerKey, job.LocalDate, sent); err != nil {
			return err
		}
	}
	return nil
}

func reminderBody(job domain.ReminderJob) string {
	body := strings.TrimSpace(job.Title)
	if job.DueTime != "" {
		body += " · " + job.DueTime
	}
	if strings.TrimSpace(job.Description) != "" {
		detail := strings.TrimSpace(job.Description)
		if len([]rune(detail)) > 100 {
			detail = string([]rune(detail)[:100]) + "…"
		}
		body += "\n" + detail
	}
	return body
}
