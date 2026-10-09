package application

import (
	"math"
	"testing"

	"avatar-id/internal/domain"
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

func TestValidateNutritionGoals(t *testing.T) {
	valid := domain.NutritionGoals{CalorieGoal: 2200, ProteinGoal: 120, FatGoal: 80, CarbohydrateGoal: 280}
	if err := ValidateNutritionGoals(valid); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, invalid := range []domain.NutritionGoals{
		{CalorieGoal: 499, ProteinGoal: 120, FatGoal: 80, CarbohydrateGoal: 280},
		{CalorieGoal: 2200, ProteinGoal: 0, FatGoal: 80, CarbohydrateGoal: 280},
		{CalorieGoal: 2200, ProteinGoal: 120, FatGoal: 1001, CarbohydrateGoal: 280},
		{CalorieGoal: 2200, ProteinGoal: 120, FatGoal: 80, CarbohydrateGoal: 0},
	} {
		if err := ValidateNutritionGoals(invalid); err == nil {
			t.Fatalf("expected error for %#v", invalid)
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
	profile, err := NormalizeProfile(domain.Profile{Name: " michael ", Surname: " foster ", Occupation: "backend-developer", DOB: "15052006", Expiry: "01.01.2030"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "MICHAEL" || profile.Surname != "FOSTER" || profile.Occupation != "BACKEND-DEVELOPER" || profile.DOB != "15.05.2006" || profile.Expiry != fixedCardExpiryDate {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

func TestNormalizeBottomNavigation(t *testing.T) {
	items, err := NormalizeBottomNavigation([]string{"calories", "card", "profile"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0] != "calories" || items[2] != "profile" {
		t.Fatalf("unexpected navigation items: %#v", items)
	}
	for _, invalid := range [][]string{
		{"card"},
		{"card", "tracker", "tracker", "profile"},
		{"card", "tasks", "unknown", "profile"},
		{"tasks", "projects", "calories", "profile"},
		{"card", "tasks", "projects", "calories"},
		{"card", "tasks", "projects", "tracker", "calories", "profile", "unknown"},
	} {
		if _, err := NormalizeBottomNavigation(invalid); err == nil {
			t.Fatalf("expected invalid navigation order to fail: %#v", invalid)
		}
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

func TestNormalizeTaskProjects(t *testing.T) {
	input, err := NormalizeTask(domain.TaskInput{
		Title: "Задача проекта", DueDate: "2026-08-24", ProjectIDs: []int64{7, 3, 7},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.ProjectIDs) != 2 || input.ProjectIDs[0] != 7 || input.ProjectIDs[1] != 3 {
		t.Fatalf("unexpected project ids: %#v", input.ProjectIDs)
	}
	if _, err := NormalizeTask(domain.TaskInput{Title: "Задача", DueDate: "2026-08-24", ProjectIDs: []int64{0}}, true); err == nil {
		t.Fatal("expected invalid project id to fail")
	}
}

func TestNormalizeTaskRecurrence(t *testing.T) {
	input, err := NormalizeTask(domain.TaskInput{
		Title: "  Зарядка  ", DueDate: "2026-08-13", RecurrenceType: " Weekly ",
		RecurrenceInterval: 2, RecurrenceEndDate: "2026-12-31", RecurrenceWeekdays: []int{5, 1, 5},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if input.RecurrenceType != "weekly" || input.RecurrenceInterval != 2 || len(input.RecurrenceWeekdays) != 2 || input.RecurrenceWeekdays[0] != 1 || input.RecurrenceWeekdays[1] != 5 {
		t.Fatalf("unexpected normalized recurrence: %#v", input)
	}
	defaultWeekday, err := NormalizeTask(domain.TaskInput{
		Title: "По четвергам", DueDate: "2026-08-13", RecurrenceType: "weekly", RecurrenceInterval: 1,
	}, true)
	if err != nil || len(defaultWeekday.RecurrenceWeekdays) != 1 || defaultWeekday.RecurrenceWeekdays[0] != 4 {
		t.Fatalf("expected due date weekday by default: %#v, %v", defaultWeekday, err)
	}
	if _, err := NormalizeTask(domain.TaskInput{
		Title: "Без даты", Category: "Дом", RecurrenceType: "daily", RecurrenceInterval: 1,
	}, true); err == nil {
		t.Fatal("recurring task without a due date must be rejected")
	}
	if _, err := NormalizeTask(domain.TaskInput{
		Title: "Неверный конец", DueDate: "2026-08-13", RecurrenceType: "monthly",
		RecurrenceInterval: 1, RecurrenceEndDate: "2026-08-12",
	}, true); err == nil {
		t.Fatal("recurrence end date before due date must be rejected")
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
