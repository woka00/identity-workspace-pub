package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"avatar-id/internal/domain"
)

type timeActivityReminderRepositoryStub struct {
	Repository
	jobs          []domain.TimeActivityReminderJob
	claimedAt     time.Time
	completedID   int64
	completedAt   time.Time
	completedSent bool
	taskClaimErr  error
}

func (repository *timeActivityReminderRepositoryStub) ClaimDueReminders(context.Context, time.Time, int) ([]domain.ReminderJob, error) {
	return nil, repository.taskClaimErr
}

func (*timeActivityReminderRepositoryStub) ClaimDueTrackerReminders(context.Context, string, string, time.Time, int) ([]domain.TrackerReminderJob, error) {
	return nil, nil
}

func (repository *timeActivityReminderRepositoryStub) ClaimDueTimeActivityReminders(_ context.Context, now time.Time, _ int) ([]domain.TimeActivityReminderJob, error) {
	repository.claimedAt = now
	return repository.jobs, nil
}

func (repository *timeActivityReminderRepositoryStub) CompleteTimeActivityReminder(_ context.Context, sessionID int64, now time.Time, sent bool) error {
	repository.completedID = sessionID
	repository.completedAt = now
	repository.completedSent = sent
	return nil
}

type timeActivityPushGatewayStub struct {
	payload []byte
}

func (*timeActivityPushGatewayStub) Configured() bool  { return true }
func (*timeActivityPushGatewayStub) PublicKey() string { return "public" }
func (gateway *timeActivityPushGatewayStub) Send(_ context.Context, _ domain.PushSubscription, payload []byte) (int, error) {
	gateway.payload = payload
	return 201, nil
}

func TestDeliverDueTimeActivityReminder(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	repository := &timeActivityReminderRepositoryStub{jobs: []domain.TimeActivityReminderJob{{
		SessionID:     12,
		ActivityID:    7,
		ActivityName:  "Работа за компьютером",
		Message:       "Разомни шею",
		Subscriptions: []domain.PushSubscription{{ID: 3}},
	}}}
	push := &timeActivityPushGatewayStub{}
	service := New(repository, nil, func() time.Time { return now }).WithPush(push)

	if err := service.deliverDueTimeActivityReminders(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !repository.claimedAt.Equal(now.UTC()) {
		t.Fatalf("claimed at %s, want %s", repository.claimedAt, now.UTC())
	}
	if repository.completedID != 12 || !repository.completedAt.Equal(now) || !repository.completedSent {
		t.Fatalf("completion = id %d at %s sent %v", repository.completedID, repository.completedAt, repository.completedSent)
	}
	var payload struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		URL      string `json:"url"`
		Renotify bool   `json:"renotify"`
	}
	if err := json.Unmarshal(push.payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Title != "Таймер · Работа за компьютером" || payload.Body != "Разомни шею" || payload.URL != "/?view=tracker" || !payload.Renotify {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestDeliverDueRemindersDoesNotStarveTimeActivityAfterTaskFailure(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	repository := &timeActivityReminderRepositoryStub{
		taskClaimErr: errors.New("task queue unavailable"),
		jobs: []domain.TimeActivityReminderJob{{
			SessionID:     12,
			ActivityID:    7,
			ActivityName:  "Работа",
			Message:       "Разомни шею",
			Subscriptions: []domain.PushSubscription{{ID: 3}},
		}},
	}
	service := New(repository, nil, func() time.Time { return now }).WithPush(&timeActivityPushGatewayStub{})

	err := service.deliverDueReminders(context.Background())
	if err == nil || !errors.Is(err, repository.taskClaimErr) {
		t.Fatalf("delivery error = %v, want task queue error", err)
	}
	if repository.completedID != 12 || !repository.completedSent {
		t.Fatalf("time reminder completion = id %d sent %v", repository.completedID, repository.completedSent)
	}
}
