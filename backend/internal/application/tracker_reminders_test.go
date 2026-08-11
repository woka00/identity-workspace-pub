package application

import (
	"testing"

	"identity-workspace/internal/domain"
)

func TestNormalizeTrackerReminder(t *testing.T) {
	valid := []struct {
		input    domain.TrackerReminderInput
		customID int64
	}{
		{domain.TrackerReminderInput{TrackerKey: "water", Time: "08:05", Enabled: true}, 0},
		{domain.TrackerReminderInput{TrackerKey: " calories ", Time: "20:30", Enabled: false}, 0},
		{domain.TrackerReminderInput{TrackerKey: "custom:42", Time: "07:00", Enabled: true}, 42},
	}
	for _, test := range valid {
		got, customID, err := normalizeTrackerReminder(test.input)
		if err != nil {
			t.Fatalf("normalizeTrackerReminder(%#v): %v", test.input, err)
		}
		if customID != test.customID {
			t.Fatalf("custom id: got %d want %d", customID, test.customID)
		}
		if got.Time == "" || got.TrackerKey == "" {
			t.Fatalf("normalization returned empty fields: %#v", got)
		}
	}

	invalid := []domain.TrackerReminderInput{
		{TrackerKey: "sleep", Time: "08:00"},
		{TrackerKey: "custom:0", Time: "08:00"},
		{TrackerKey: "custom:nope", Time: "08:00"},
		{TrackerKey: "water", Time: "8:00"},
		{TrackerKey: "water", Time: "25:00"},
	}
	for _, input := range invalid {
		if _, _, err := normalizeTrackerReminder(input); err == nil {
			t.Fatalf("expected invalid reminder to fail: %#v", input)
		}
	}
}
