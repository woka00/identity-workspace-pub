package postgres

import (
	"testing"
	"time"
)

func TestSplitTimeSessionAcrossLocalMidnight(t *testing.T) {
	location := time.FixedZone("MSK", 3*60*60)
	start := time.Date(2026, time.August, 28, 23, 30, 0, 0, location)
	end := time.Date(2026, time.August, 29, 0, 30, 0, 0, location)
	session := storedTimeSession{
		id:           1,
		activityID:   2,
		activityName: "Спорт",
		startedAt:    start,
		endedAt:      end,
	}

	segments := splitTimeSession(session, start, end, location)
	if len(segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(segments))
	}
	if segments[0].Date != "2026-08-28" || segments[1].Date != "2026-08-29" {
		t.Fatalf("dates = %s, %s", segments[0].Date, segments[1].Date)
	}
	if segments[0].DurationSeconds != 1800 || segments[1].DurationSeconds != 1800 {
		t.Fatalf("durations = %d, %d", segments[0].DurationSeconds, segments[1].DurationSeconds)
	}
}
