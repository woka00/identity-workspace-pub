package application

import (
	"testing"

	"avatar-id/internal/domain"
)

func TestCustomTrackerStateForDateResetsMissingDailyValues(t *testing.T) {
	state := domain.TrackerState{
		CustomTrackers: []domain.CustomTracker{
			{ID: 1, CurrentValue: 8, UpdatedAt: "old"},
			{ID: 2, CurrentValue: 4, UpdatedAt: "old"},
		},
		CustomHistory: []domain.CustomTrackerEntry{
			{TrackerID: 1, Date: "2026-08-18", Value: 8, UpdatedAt: "yesterday"},
			{TrackerID: 1, Date: "2026-08-19", Value: 2, UpdatedAt: "today"},
		},
	}

	got := customTrackerStateForDate(state, "2026-08-19")
	if got.CustomTrackers[0].CurrentValue != 2 || got.CustomTrackers[0].UpdatedAt != "today" {
		t.Fatalf("today value was not selected: %#v", got.CustomTrackers[0])
	}
	if got.CustomTrackers[1].CurrentValue != 0 {
		t.Fatalf("tracker without a daily entry=%v, want 0", got.CustomTrackers[1].CurrentValue)
	}
}
