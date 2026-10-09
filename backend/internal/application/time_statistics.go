package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"avatar-id/internal/domain"
)

type timeStatisticsRepository interface {
	TimeStatistics(context.Context, domain.TimeTrackerRange) (domain.TimeStatistics, error)
	TimeActivityStatistics(context.Context, int64, domain.TimeActivityStatisticsRange) (domain.TimeActivityStatistics, error)
}

func (s *Service) timeStatisticsRepository() (timeStatisticsRepository, error) {
	repository, ok := s.repo.(timeStatisticsRepository)
	if !ok {
		return nil, fmt.Errorf("time statistics repository is unavailable: %w", domain.ErrConflict)
	}
	return repository, nil
}

func normalizeTimeStatisticsPeriod(period string) (string, error) {
	period = strings.ToLower(strings.TrimSpace(period))
	if period == "" {
		return "today", nil
	}
	if period != "today" && period != "week" && period != "month" {
		return "", invalidf("time statistics period must be today, week or month")
	}
	return period, nil
}

func timeStatisticsRange(now time.Time, period string) domain.TimeTrackerRange {
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	periodStart := todayStart
	switch period {
	case "week":
		periodStart = todayStart.AddDate(0, 0, -6)
	case "month":
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
	return domain.TimeTrackerRange{
		Date:          todayStart.Format("2006-01-02"),
		Period:        period,
		TodayStart:    todayStart,
		TomorrowStart: todayStart.AddDate(0, 0, 1),
		PeriodStart:   periodStart,
		PeriodEnd:     todayStart.AddDate(0, 0, 1),
		Now:           now,
	}
}

func (s *Service) TimeStatistics(ctx context.Context, period string) (domain.TimeStatistics, error) {
	period, err := normalizeTimeStatisticsPeriod(period)
	if err != nil {
		return domain.TimeStatistics{}, err
	}
	repository, err := s.timeStatisticsRepository()
	if err != nil {
		return domain.TimeStatistics{}, err
	}
	return repository.TimeStatistics(ctx, timeStatisticsRange(s.now(), period))
}

func (s *Service) TimeActivityStatistics(ctx context.Context, id int64) (domain.TimeActivityStatistics, error) {
	if id <= 0 {
		return domain.TimeActivityStatistics{}, invalidf("invalid time activity id")
	}
	repository, err := s.timeStatisticsRepository()
	if err != nil {
		return domain.TimeActivityStatistics{}, err
	}
	now := s.now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return repository.TimeActivityStatistics(ctx, id, domain.TimeActivityStatisticsRange{
		Now:       now,
		WeekStart: todayStart.AddDate(0, 0, -6),
	})
}
