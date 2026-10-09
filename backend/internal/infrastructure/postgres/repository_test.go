package postgres

import (
	"reflect"
	"testing"
)

func TestRecurrenceWeekdaysMaskRoundTrip(t *testing.T) {
	weekdays := []int{1, 3, 5, 7}
	mask := recurrenceWeekdaysMask(weekdays)
	if mask != 85 {
		t.Fatalf("unexpected weekday mask: %d", mask)
	}
	if restored := recurrenceWeekdaysFromMask(mask); !reflect.DeepEqual(restored, weekdays) {
		t.Fatalf("unexpected restored weekdays: %#v", restored)
	}
}

func TestNextCustomTrackerValueUsesDailyValueAndBounds(t *testing.T) {
	if got := nextCustomTrackerValue(0, 2.5, 10, 1); got != 2.5 {
		t.Fatalf("first daily step=%v, want 2.5", got)
	}
	if got := nextCustomTrackerValue(9, 2.5, 10, 1); got != 10 {
		t.Fatalf("upper bound=%v, want 10", got)
	}
	if got := nextCustomTrackerValue(1, 2.5, 10, -1); got != 0 {
		t.Fatalf("lower bound=%v, want 0", got)
	}
}
