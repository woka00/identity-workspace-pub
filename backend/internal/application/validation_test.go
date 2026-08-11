package application

import (
	"math"
	"testing"

	"identity-workspace/internal/domain"
)

func TestNormalizeTaskDefaultsToTodo(t *testing.T) {
	input, err := NormalizeTask(domain.TaskInput{
		Title: "  Подготовить релиз  ", Category: " Backend ", DueDate: " 2026-08-07 ",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if input.Title != "Подготовить релиз" || input.Category != "Backend" || input.Status != "todo" || input.DueDate != "2026-08-07" {
		t.Fatalf("unexpected task input: %#v", input)
	}
}

func TestNormalizeTaskRejectsInvalidStatuses(t *testing.T) {
	for _, tc := range []struct {
		create bool
		status string
	}{
		{true, "done"}, {true, "doing"}, {false, "doing"},
	} {
		_, err := NormalizeTask(domain.TaskInput{Title: "Задача", Status: tc.status, DueDate: "2026-08-07"}, tc.create)
		if err == nil {
			t.Fatalf("expected error for create=%v status=%q", tc.create, tc.status)
		}
	}
}

func TestNormalizeTaskAllowsBinaryUpdateStatuses(t *testing.T) {
	for _, status := range []string{"todo", "done"} {
		input, err := NormalizeTask(domain.TaskInput{Title: "Задача", Status: status, DueDate: "2026-08-07"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if input.Status != status {
			t.Fatalf("expected %q, got %q", status, input.Status)
		}
	}
}

func TestNormalizeTaskCategoryAndDateRules(t *testing.T) {
	if _, err := NormalizeTask(domain.TaskInput{Title: "Без даты", Category: " Работа "}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeTask(domain.TaskInput{Title: "Без даты"}, true); err == nil {
		t.Fatal("expected category requirement")
	}
	for _, date := range []string{"03.08.2026", "2026-02-30", "2026-08-03T12:00:00Z", "завтра"} {
		if _, err := NormalizeTask(domain.TaskInput{Title: "Задача", DueDate: date}, true); err == nil {
			t.Fatalf("expected invalid date error for %q", date)
		}
	}
}

func TestNormalizeDate(t *testing.T) {
	date, err := NormalizeDate(" 2026-08-03 ", "tracker date")
	if err != nil {
		t.Fatal(err)
	}
	if date != "2026-08-03" {
		t.Fatalf("unexpected date: %q", date)
	}
	for _, value := range []string{"", "03.08.2026", "2026-02-30", "2026-8-3"} {
		if _, err := NormalizeDate(value, "tracker date"); err == nil {
			t.Fatalf("expected validation error for %q", value)
		}
	}
}

func TestNormalizeWeight(t *testing.T) {
	value, err := NormalizeWeight(91.56)
	if err != nil {
		t.Fatal(err)
	}
	if value != 91.6 {
		t.Fatalf("expected 91.6, got %v", value)
	}
	for _, invalid := range []float64{19.9, 500.1, math.NaN(), math.Inf(1)} {
		if _, err := NormalizeWeight(invalid); err == nil {
			t.Fatalf("expected error for %v", invalid)
		}
	}
}

func TestValidateWater(t *testing.T) {
	if err := ValidateWater(7, 9); err != nil {
		t.Fatal(err)
	}
	for _, input := range [][2]int{{-1, 8}, {100, 8}, {1, 0}, {1, 31}} {
		if err := ValidateWater(input[0], input[1]); err == nil {
			t.Fatalf("expected error for %v", input)
		}
	}
}

func TestValidateCalorieGoal(t *testing.T) {
	for _, valid := range []int{500, 2000, 10000} {
		if err := ValidateCalorieGoal(valid); err != nil {
			t.Fatalf("unexpected error for %d: %v", valid, err)
		}
	}
	for _, invalid := range []int{0, 499, 10001} {
		if err := ValidateCalorieGoal(invalid); err == nil {
			t.Fatalf("expected error for %d", invalid)
		}
	}
}

func TestNormalizeGoalRules(t *testing.T) {
	_, err := NormalizeGoal(domain.GoalInput{Title: "Запуск", TargetValue: 1, Pinned: true})
	if err == nil {
		t.Fatal("expected pinned active project error")
	}

	input, err := NormalizeGoal(domain.GoalInput{
		Title: "Запуск", TargetValue: 1, RelatedTaskIDs: []string{"1", "1", "2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.RelatedTaskIDs) != 2 || input.RelatedTaskIDs[0] != "1" || input.RelatedTaskIDs[1] != "2" {
		t.Fatalf("unexpected related IDs: %#v", input.RelatedTaskIDs)
	}
}

func TestNormalizeProfile(t *testing.T) {
	profile, err := NormalizeProfile(domain.Profile{Name: " michael ", Surname: " foster ", Occupation: "backend-developer"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "MICHAEL" || profile.Surname != "FOSTER" || profile.Occupation != "BACKEND-DEVELOPER" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

func TestNormalizeTaskPlanningFields(t *testing.T) {
	input, err := NormalizeTask(domain.TaskInput{
		Title:      "Встреча",
		Category:   "Работа",
		DueDate:    "2026-08-08",
		DueTime:    "14:30",
		ReminderAt: "2026-08-08T10:20:00+03:00",
		Priority:   3,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if input.DueTime != "14:30" || input.ReminderAt != "2026-08-08T07:20:00Z" || input.Priority != 3 || !input.IsMilestone {
		t.Fatalf("unexpected planning fields: %#v", input)
	}
	if _, err := NormalizeTask(domain.TaskInput{Title: "Без даты", Category: "Дом", DueTime: "09:00"}, true); err == nil {
		t.Fatal("expected due time without date to fail")
	}
	if _, err := NormalizeTask(domain.TaskInput{Title: "Приоритет", Category: "Дом", Priority: 4}, true); err == nil {
		t.Fatal("expected invalid priority to fail")
	}
}

func TestNormalizeCustomTracker(t *testing.T) {
	input, err := NormalizeCustomTracker(domain.CustomTrackerInput{
		Name: "  Прочитать книг  ", TargetValue: 12.3456, StepValue: 1, Icon: "Book",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "Прочитать книг" || input.Icon != "book" || input.TargetValue != 12.346 || input.StepValue != 1 {
		t.Fatalf("unexpected tracker input: %#v", input)
	}
	if _, err := NormalizeCustomTracker(domain.CustomTrackerInput{Name: "Иконка", TargetValue: 10, StepValue: 1, Icon: "icon-36"}); err != nil {
		t.Fatalf("new PNG tracker icon must be accepted: %v", err)
	}
	for _, invalid := range []domain.CustomTrackerInput{
		{Name: "", TargetValue: 10, StepValue: 1, Icon: "book"},
		{Name: "Тест", TargetValue: 0, StepValue: 1, Icon: "book"},
		{Name: "Тест", TargetValue: 10, StepValue: 11, Icon: "book"},
		{Name: "Тест", TargetValue: 10, StepValue: 1, Icon: "unknown"},
	} {
		if _, err := NormalizeCustomTracker(invalid); err == nil {
			t.Fatalf("expected invalid tracker to fail: %#v", invalid)
		}
	}
}
