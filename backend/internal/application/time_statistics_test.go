package application

import (
	"context"
	"testing"
	"time"

	"avatar-id/internal/domain"
)

type timeStatisticsRepositoryStub struct {
	Repository
	rangeValue         domain.TimeTrackerRange
	activityID         int64
	activityRangeValue domain.TimeActivityStatisticsRange
}

func (repository *timeStatisticsRepositoryStub) TimeStatistics(_ context.Context, value domain.TimeTrackerRange) (domain.TimeStatistics, error) {
	repository.rangeValue = value
	return domain.TimeStatistics{Date: value.Date, Period: value.Period}, nil
}

func (repository *timeStatisticsRepositoryStub) TimeActivityStatistics(_ context.Context, id int64, value domain.TimeActivityStatisticsRange) (domain.TimeActivityStatistics, error) {
	repository.activityID = id
	repository.activityRangeValue = value
	return domain.TimeActivityStatistics{ActivityID: id}, nil
}

func TestTimeStatisticsWeekCoversLastSevenDays(t *testing.T) {
	location := time.FixedZone("MSK", 3*60*60)
	now := time.Date(2026, time.August, 29, 14, 30, 0, 0, location)
	repository := &timeStatisticsRepositoryStub{}
	service := New(repository, nil, func() time.Time { return now })

	if _, err := service.TimeStatistics(context.Background(), "week"); err != nil {
		t.Fatal(err)
	}
	if got := repository.rangeValue.PeriodStart.Format("2006-01-02"); got != "2026-08-23" {
		t.Fatalf("period start = %s, want 2026-08-23", got)
	}
	if got := repository.rangeValue.PeriodEnd.Format("2006-01-02"); got != "2026-08-30" {
		t.Fatalf("period end = %s, want 2026-08-30", got)
	}
}

func TestTimeStatisticsMonthStartsOnFirstDay(t *testing.T) {
	location := time.FixedZone("MSK", 3*60*60)
	now := time.Date(2026, time.August, 29, 14, 30, 0, 0, location)
	repository := &timeStatisticsRepositoryStub{}
	service := New(repository, nil, func() time.Time { return now })

	if _, err := service.TimeStatistics(context.Background(), "month"); err != nil {
		t.Fatal(err)
	}
	if got := repository.rangeValue.PeriodStart.Format("2006-01-02"); got != "2026-08-01" {
		t.Fatalf("period start = %s, want 2026-08-01", got)
	}
}

func TestTimeActivityStatisticsUsesRollingWeek(t *testing.T) {
	now := time.Date(2026, time.August, 29, 14, 30, 0, 0, time.UTC)
	repository := &timeStatisticsRepositoryStub{}
	service := New(repository, nil, func() time.Time { return now })

	if _, err := service.TimeActivityStatistics(context.Background(), 12); err != nil {
		t.Fatal(err)
	}
	if repository.activityID != 12 {
		t.Fatalf("activity id = %d, want 12", repository.activityID)
	}
	if got := repository.activityRangeValue.WeekStart.Format("2006-01-02"); got != "2026-08-23" {
		t.Fatalf("week start = %s, want 2026-08-23", got)
	}
}
