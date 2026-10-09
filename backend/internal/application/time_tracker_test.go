package application

import (
	"context"
	"testing"
	"time"

	"avatar-id/internal/domain"
)

type timeTrackerRepositoryStub struct {
	Repository
	rangeValue domain.TimeTrackerRange
	startedID  int64
	startedAt  time.Time
	stoppedAt  time.Time
	pausedAt   time.Time
	updatedID  int64
	updatedAt  time.Time
	updated    domain.TimeActivityInput
	deletedID  int64
	deletedAt  time.Time
}

func (repository *timeTrackerRepositoryStub) TimeTracker(_ context.Context, value domain.TimeTrackerRange) (domain.TimeTrackerState, error) {
	repository.rangeValue = value
	return domain.TimeTrackerState{Date: value.Date, Period: value.Period, Activities: []domain.TimeActivity{}}, nil
}

func (*timeTrackerRepositoryStub) CreateTimeActivity(_ context.Context, input domain.TimeActivityInput) (domain.TimeActivity, error) {
	return domain.TimeActivity{ID: 1, Name: input.Name, ReminderIntervalMinutes: input.ReminderIntervalMinutes, ReminderMessage: input.ReminderMessage}, nil
}

func (repository *timeTrackerRepositoryStub) UpdateTimeActivity(_ context.Context, id int64, input domain.TimeActivityInput, now time.Time) (domain.TimeActivity, error) {
	repository.updatedID = id
	repository.updatedAt = now
	repository.updated = input
	return domain.TimeActivity{ID: id, Name: input.Name, ReminderIntervalMinutes: input.ReminderIntervalMinutes, ReminderMessage: input.ReminderMessage}, nil
}

func (repository *timeTrackerRepositoryStub) DeleteTimeActivity(_ context.Context, id int64, now time.Time) error {
	repository.deletedID = id
	repository.deletedAt = now
	return nil
}

func TestTimeActivityReminderNormalization(t *testing.T) {
	repository := &timeTrackerRepositoryStub{}
	service := New(repository, nil, time.Now)

	activity, err := service.CreateTimeActivity(context.Background(), domain.TimeActivityInput{
		Name:                    "  Работа  ",
		ReminderIntervalMinutes: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if activity.Name != "Работа" || activity.ReminderMessage != defaultTimeActivityReminderMessage {
		t.Fatalf("normalized activity = %#v", activity)
	}

	invalidIntervals := []int{-1, 1, 4, 241}
	for _, interval := range invalidIntervals {
		_, err := service.CreateTimeActivity(context.Background(), domain.TimeActivityInput{
			Name:                    "Работа",
			ReminderIntervalMinutes: interval,
		})
		if err == nil {
			t.Fatalf("interval %d must be rejected", interval)
		}
	}
}

func TestUpdateTimeActivityUsesServiceClock(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	repository := &timeTrackerRepositoryStub{}
	service := New(repository, nil, func() time.Time { return now })

	_, err := service.UpdateTimeActivity(context.Background(), 7, domain.TimeActivityInput{
		Name:                    "Компьютер",
		ReminderIntervalMinutes: 45,
		ReminderMessage:         "  Разомни шею  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.updatedID != 7 || !repository.updatedAt.Equal(now) {
		t.Fatalf("update = id %d at %s", repository.updatedID, repository.updatedAt)
	}
	if repository.updated.ReminderMessage != "Разомни шею" {
		t.Fatalf("message = %q", repository.updated.ReminderMessage)
	}
}

func TestDeleteTimeActivityUsesServiceClock(t *testing.T) {
	now := time.Date(2026, time.September, 3, 13, 0, 0, 0, time.UTC)
	repository := &timeTrackerRepositoryStub{}
	service := New(repository, nil, func() time.Time { return now })

	if err := service.DeleteTimeActivity(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	if repository.deletedID != 9 || !repository.deletedAt.Equal(now) {
		t.Fatalf("delete = id %d at %s", repository.deletedID, repository.deletedAt)
	}
	if err := service.DeleteTimeActivity(context.Background(), 0); err == nil {
		t.Fatal("invalid activity id must be rejected")
	}
}

func (repository *timeTrackerRepositoryStub) StartTimeActivity(_ context.Context, id int64, now time.Time) error {
	repository.startedID = id
	repository.startedAt = now
	return nil
}

func (repository *timeTrackerRepositoryStub) StopTimeActivity(_ context.Context, now time.Time) error {
	repository.stoppedAt = now
	return nil
}

func TestTimeTrackerWeekStartsOnMonday(t *testing.T) {
	location := time.FixedZone("MSK", 3*60*60)
	now := time.Date(2026, time.August, 29, 14, 30, 0, 0, location)
	repository := &timeTrackerRepositoryStub{}
	service := New(repository, nil, func() time.Time { return now })

	state, err := service.TimeTracker(context.Background(), "week")
	if err != nil {
		t.Fatal(err)
	}
	if state.Date != "2026-08-29" || state.Period != "week" {
		t.Fatalf("state = %#v", state)
	}
	if got := repository.rangeValue.PeriodStart.Format("2006-01-02"); got != "2026-08-24" {
		t.Fatalf("period start = %s, want Monday 2026-08-24", got)
	}
}

func TestStartTimeActivityUsesServiceClock(t *testing.T) {
	now := time.Date(2026, time.August, 29, 14, 30, 0, 0, time.UTC)
	repository := &timeTrackerRepositoryStub{}
	service := New(repository, nil, func() time.Time { return now })

	if _, err := service.StartTimeActivity(context.Background(), 7, "today"); err != nil {
		t.Fatal(err)
	}
	if repository.startedID != 7 || !repository.startedAt.Equal(now) {
		t.Fatalf("start = id %d at %s", repository.startedID, repository.startedAt)
	}
}

func (repository *timeTrackerRepositoryStub) PauseTimeActivity(_ context.Context, now time.Time) error {
	repository.pausedAt = now
	return nil
}

func TestPauseTimeActivityDoesNotFinish(t *testing.T) {
	now := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	repository := &timeTrackerRepositoryStub{}
	service := New(repository, nil, func() time.Time { return now })
	if _, err := service.PauseTimeActivity(context.Background(), "invalid"); err == nil {
		t.Fatal("invalid period accepted")
	}
	if !repository.pausedAt.IsZero() {
		t.Fatal("invalid request mutated timer")
	}
	if _, err := service.PauseTimeActivity(context.Background(), "week"); err != nil {
		t.Fatal(err)
	}
	if !repository.pausedAt.Equal(now) || !repository.stoppedAt.IsZero() {
		t.Fatal("pause must use service clock without finishing")
	}
	if repository.rangeValue.Period != "week" {
		t.Fatal("pause lost selected statistics period")
	}
	if _, err := service.StopTimeActivity(context.Background(), "today"); err != nil {
		t.Fatal(err)
	}
	if !repository.stoppedAt.Equal(now) {
		t.Fatal("finish did not stop timer")
	}
}
