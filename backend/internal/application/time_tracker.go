package application

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"avatar-id/internal/domain"
)

type timeTrackerRepository interface {
	TimeTracker(context.Context, domain.TimeTrackerRange) (domain.TimeTrackerState, error)
	CreateTimeActivity(context.Context, domain.TimeActivityInput) (domain.TimeActivity, error)
	UpdateTimeActivity(context.Context, int64, domain.TimeActivityInput, time.Time) (domain.TimeActivity, error)
	DeleteTimeActivity(context.Context, int64, time.Time) error
	StartTimeActivity(context.Context, int64, time.Time) error
	StopTimeActivity(context.Context, time.Time) error
	PauseTimeActivity(context.Context, time.Time) error
}

const defaultTimeActivityReminderMessage = "Пора сделать перерыв и немного подвигаться"

func normalizeTimeActivityInput(input domain.TimeActivityInput) (domain.TimeActivityInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 60 {
		return domain.TimeActivityInput{}, invalidf("time activity name must be 1..60 characters")
	}
	input.ReminderMessage = strings.TrimSpace(input.ReminderMessage)
	if input.ReminderIntervalMinutes == 0 {
		input.ReminderMessage = ""
		return input, nil
	}
	if input.ReminderIntervalMinutes < 5 || input.ReminderIntervalMinutes > 240 {
		return domain.TimeActivityInput{}, invalidf("time activity reminder interval must be 5..240 minutes")
	}
	if input.ReminderMessage == "" {
		input.ReminderMessage = defaultTimeActivityReminderMessage
	}
	if utf8.RuneCountInString(input.ReminderMessage) > 160 {
		return domain.TimeActivityInput{}, invalidf("time activity reminder message must be at most 160 characters")
	}
	return input, nil
}

func (s *Service) timeTrackerRepository() (timeTrackerRepository, error) {
	repository, ok := s.repo.(timeTrackerRepository)
	if !ok {
		return nil, fmt.Errorf("time tracker repository is unavailable: %w", domain.ErrConflict)
	}
	return repository, nil
}

func normalizeTimeTrackerPeriod(period string) (string, error) {
	period = strings.ToLower(strings.TrimSpace(period))
	if period == "" {
		return "today", nil
	}
	if period != "today" && period != "week" {
		return "", invalidf("time tracker period must be today or week")
	}
	return period, nil
}

func timeTrackerRange(now time.Time, period string) domain.TimeTrackerRange {
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	periodStart := todayStart
	if period == "week" {
		daysSinceMonday := (int(todayStart.Weekday()) + 6) % 7
		periodStart = todayStart.AddDate(0, 0, -daysSinceMonday)
	}
	return domain.TimeTrackerRange{
		Date:          todayStart.Format("2006-01-02"),
		Period:        period,
		TodayStart:    todayStart,
		TomorrowStart: tomorrowStart,
		PeriodStart:   periodStart,
		PeriodEnd:     tomorrowStart,
		Now:           now,
	}
}

func (s *Service) TimeTracker(ctx context.Context, period string) (domain.TimeTrackerState, error) {
	period, err := normalizeTimeTrackerPeriod(period)
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	repository, err := s.timeTrackerRepository()
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	return repository.TimeTracker(ctx, timeTrackerRange(s.now(), period))
}

func (s *Service) CreateTimeActivity(ctx context.Context, input domain.TimeActivityInput) (domain.TimeActivity, error) {
	input, err := normalizeTimeActivityInput(input)
	if err != nil {
		return domain.TimeActivity{}, err
	}
	repository, err := s.timeTrackerRepository()
	if err != nil {
		return domain.TimeActivity{}, err
	}
	return repository.CreateTimeActivity(ctx, input)
}

func (s *Service) UpdateTimeActivity(ctx context.Context, id int64, input domain.TimeActivityInput) (domain.TimeActivity, error) {
	if id <= 0 {
		return domain.TimeActivity{}, invalidf("invalid time activity id")
	}
	input, err := normalizeTimeActivityInput(input)
	if err != nil {
		return domain.TimeActivity{}, err
	}
	repository, err := s.timeTrackerRepository()
	if err != nil {
		return domain.TimeActivity{}, err
	}
	return repository.UpdateTimeActivity(ctx, id, input, s.now())
}

func (s *Service) DeleteTimeActivity(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid time activity id")
	}
	repository, err := s.timeTrackerRepository()
	if err != nil {
		return err
	}
	return repository.DeleteTimeActivity(ctx, id, s.now())
}

func (s *Service) StartTimeActivity(ctx context.Context, id int64, period string) (domain.TimeTrackerState, error) {
	if id <= 0 {
		return domain.TimeTrackerState{}, invalidf("invalid time activity id")
	}
	period, err := normalizeTimeTrackerPeriod(period)
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	repository, err := s.timeTrackerRepository()
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	now := s.now()
	if err := repository.StartTimeActivity(ctx, id, now); err != nil {
		return domain.TimeTrackerState{}, err
	}
	return repository.TimeTracker(ctx, timeTrackerRange(now, period))
}

func (s *Service) StopTimeActivity(ctx context.Context, period string) (domain.TimeTrackerState, error) {
	period, err := normalizeTimeTrackerPeriod(period)
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	repository, err := s.timeTrackerRepository()
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	now := s.now()
	if err := repository.StopTimeActivity(ctx, now); err != nil {
		return domain.TimeTrackerState{}, err
	}
	return repository.TimeTracker(ctx, timeTrackerRange(now, period))
}

func (s *Service) PauseTimeActivity(ctx context.Context, period string) (domain.TimeTrackerState, error) {
	period, err := normalizeTimeTrackerPeriod(period)
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	repository, err := s.timeTrackerRepository()
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	now := s.now()
	if err := repository.PauseTimeActivity(ctx, now); err != nil {
		return domain.TimeTrackerState{}, err
	}
	return repository.TimeTracker(ctx, timeTrackerRange(now, period))
}
