package application

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"avatar-id/internal/domain"
)

func NormalizeTask(input domain.TaskInput, create bool) (domain.TaskInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Category = strings.TrimSpace(input.Category)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.DueDate = strings.TrimSpace(input.DueDate)
	input.DueTime = strings.TrimSpace(input.DueTime)
	input.ReminderAt = strings.TrimSpace(input.ReminderAt)
	input.RecurrenceType = strings.ToLower(strings.TrimSpace(input.RecurrenceType))
	input.RecurrenceEndDate = strings.TrimSpace(input.RecurrenceEndDate)
	if len(input.ProjectIDs) > 100 {
		return domain.TaskInput{}, invalidf("task can be attached to at most 100 projects")
	}
	projectIDs := make([]int64, 0, len(input.ProjectIDs))
	seenProjectIDs := make(map[int64]struct{}, len(input.ProjectIDs))
	for _, projectID := range input.ProjectIDs {
		if projectID <= 0 {
			return domain.TaskInput{}, invalidf("task projectIds must contain positive ids")
		}
		if _, exists := seenProjectIDs[projectID]; exists {
			continue
		}
		seenProjectIDs[projectID] = struct{}{}
		projectIDs = append(projectIDs, projectID)
	}
	input.ProjectIDs = projectIDs
	if create && input.Status != "" && input.Status != "todo" {
		return domain.TaskInput{}, invalidf("new task status must be todo")
	}
	if create {
		input.Status = "todo"
	}
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 160 {
		return domain.TaskInput{}, invalidf("task title must be 1..160 characters")
	}
	if utf8.RuneCountInString(input.Description) > 2000 {
		return domain.TaskInput{}, invalidf("task description must be 0..2000 characters")
	}
	if utf8.RuneCountInString(input.Category) > 40 {
		return domain.TaskInput{}, invalidf("task category must be 0..40 characters")
	}
	if input.Status != "todo" && input.Status != "done" {
		return domain.TaskInput{}, invalidf("task status must be todo or done")
	}
	if input.DueDate != "" {
		if _, err := NormalizeDate(input.DueDate, "task dueDate"); err != nil {
			return domain.TaskInput{}, err
		}
	}
	if input.DueTime != "" {
		parsed, err := time.Parse("15:04", input.DueTime)
		if err != nil || parsed.Format("15:04") != input.DueTime || input.DueDate == "" {
			return domain.TaskInput{}, invalidf("task dueTime must be HH:MM and requires dueDate")
		}
	}
	if input.ReminderAt != "" {
		parsed, err := time.Parse(time.RFC3339, input.ReminderAt)
		if err != nil {
			return domain.TaskInput{}, invalidf("task reminderAt must be RFC3339")
		}
		input.ReminderAt = parsed.UTC().Format(time.RFC3339)
	}
	if input.Priority < 0 || input.Priority > 3 {
		return domain.TaskInput{}, invalidf("task priority must be 0..3")
	}
	if input.RecurrenceType == "" {
		input.RecurrenceInterval = 1
		input.RecurrenceEndDate = ""
		input.RecurrenceWeekdays = []int{}
	} else {
		if input.RecurrenceType != "daily" && input.RecurrenceType != "weekly" && input.RecurrenceType != "monthly" {
			return domain.TaskInput{}, invalidf("task recurrenceType must be daily, weekly or monthly")
		}
		if input.DueDate == "" {
			return domain.TaskInput{}, invalidf("recurring task requires dueDate")
		}
		if input.RecurrenceInterval < 1 || input.RecurrenceInterval > 365 {
			return domain.TaskInput{}, invalidf("task recurrenceInterval must be 1..365")
		}
		if input.RecurrenceEndDate != "" {
			if _, err := NormalizeDate(input.RecurrenceEndDate, "task recurrenceEndDate"); err != nil {
				return domain.TaskInput{}, err
			}
			if input.RecurrenceEndDate < input.DueDate {
				return domain.TaskInput{}, invalidf("task recurrenceEndDate must not precede dueDate")
			}
		}
		if input.RecurrenceType == "weekly" {
			weekdays := make([]int, 0, 7)
			seen := [8]bool{}
			for _, weekday := range input.RecurrenceWeekdays {
				if weekday < 1 || weekday > 7 {
					return domain.TaskInput{}, invalidf("task recurrenceWeekdays must contain days 1..7")
				}
				seen[weekday] = true
			}
			for weekday := 1; weekday <= 7; weekday++ {
				if seen[weekday] {
					weekdays = append(weekdays, weekday)
				}
			}
			if len(weekdays) == 0 {
				dueDate, _ := time.Parse("2006-01-02", input.DueDate)
				weekday := int(dueDate.Weekday())
				if weekday == 0 {
					weekday = 7
				}
				weekdays = []int{weekday}
			}
			input.RecurrenceWeekdays = weekdays
		} else {
			input.RecurrenceWeekdays = []int{}
		}
	}
	input.IsMilestone = input.Priority == 3
	if input.DueDate == "" && input.Category == "" {
		return domain.TaskInput{}, invalidf("task category is required when dueDate is empty")
	}
	return input, nil
}

func NormalizeDate(raw, field string) (string, error) {
	raw = strings.TrimSpace(raw)
	date, err := time.Parse("2006-01-02", raw)
	if err != nil || date.Format("2006-01-02") != raw {
		return "", invalidf("%s must be YYYY-MM-DD", field)
	}
	return raw, nil
}

func NormalizeWeight(weightKg float64) (float64, error) {
	if weightKg < 20 || weightKg > 500 || math.IsNaN(weightKg) || math.IsInf(weightKg, 0) {
		return 0, invalidf("weightKg must be between 20 and 500")
	}
	return math.Round(weightKg*10) / 10, nil
}

func ValidateWater(glasses, goalGlasses int) error {
	if glasses < 0 || glasses > 99 {
		return invalidf("glasses must be between 0 and 99")
	}
	if goalGlasses < 1 || goalGlasses > 30 {
		return invalidf("goalGlasses must be between 1 and 30")
	}
	return nil
}

func ValidateCalorieGoal(calorieGoal int) error {
	if calorieGoal < 500 || calorieGoal > 10000 {
		return invalidf("calorieGoal must be between 500 and 10000")
	}
	return nil
}

func ValidateNutritionGoals(goals domain.NutritionGoals) error {
	if err := ValidateCalorieGoal(goals.CalorieGoal); err != nil {
		return err
	}
	if goals.ProteinGoal < 1 || goals.ProteinGoal > 1000 {
		return invalidf("proteinGoal must be between 1 and 1000")
	}
	if goals.FatGoal < 1 || goals.FatGoal > 1000 {
		return invalidf("fatGoal must be between 1 and 1000")
	}
	if goals.CarbohydrateGoal < 1 || goals.CarbohydrateGoal > 1000 {
		return invalidf("carbohydrateGoal must be between 1 and 1000")
	}
	return nil
}

func NormalizeGoal(input domain.GoalInput) (domain.GoalInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Summary = strings.TrimSpace(input.Summary)
	input.Unit = strings.TrimSpace(input.Unit)
	input.Deadline = strings.TrimSpace(input.Deadline)

	if input.Title == "" || utf8.RuneCountInString(input.Title) > 80 {
		return domain.GoalInput{}, invalidf("project title must be 1..80 characters")
	}
	if utf8.RuneCountInString(input.Description) > 1200 {
		return domain.GoalInput{}, invalidf("project description must be 0..1200 characters")
	}
	if utf8.RuneCountInString(input.Summary) > 180 {
		return domain.GoalInput{}, invalidf("project summary must be 0..180 characters")
	}
	if utf8.RuneCountInString(input.Unit) > 16 {
		return domain.GoalInput{}, invalidf("project unit must be 0..16 characters")
	}
	if input.CurrentValue < 0 || input.TargetValue <= 0 || input.CurrentValue > 1_000_000_000 || input.TargetValue > 1_000_000_000 ||
		math.IsNaN(input.CurrentValue) || math.IsNaN(input.TargetValue) || math.IsInf(input.CurrentValue, 0) || math.IsInf(input.TargetValue, 0) {
		return domain.GoalInput{}, invalidf("project progress values are invalid")
	}
	if input.Deadline != "" {
		if _, err := NormalizeDate(input.Deadline, "project deadline"); err != nil {
			return domain.GoalInput{}, err
		}
	}
	if input.Pinned && !input.Completed {
		return domain.GoalInput{}, invalidf("only a completed project can be pinned")
	}

	seen := make(map[string]bool, len(input.RelatedTaskIDs))
	related := make([]string, 0, len(input.RelatedTaskIDs))
	for _, raw := range input.RelatedTaskIDs {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			continue
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return domain.GoalInput{}, invalidf("related task id %q is invalid", raw)
		}
		seen[raw] = true
		related = append(related, raw)
	}
	input.RelatedTaskIDs = related
	return input, nil
}

func NormalizeProfile(profile domain.Profile) (domain.Profile, error) {
	profile.Name = strings.ToUpper(strings.TrimSpace(profile.Name))
	profile.Surname = strings.ToUpper(strings.TrimSpace(profile.Surname))
	profile.Occupation = strings.ToUpper(strings.TrimSpace(profile.Occupation))
	profile.Sex = strings.ToUpper(strings.TrimSpace(profile.Sex))
	profile.DOB = normalizeCompactProfileDate(profile.DOB)
	profile.Expiry = fixedCardExpiryDate
	if profile.Name == "" || utf8.RuneCountInString(profile.Name) > 24 || utf8.RuneCountInString(profile.Surname) > 28 ||
		utf8.RuneCountInString(profile.Occupation) > 32 || utf8.RuneCountInString(profile.Sex) > 16 ||
		utf8.RuneCountInString(profile.DOB) > 16 || utf8.RuneCountInString(profile.Expiry) > 16 {
		return domain.Profile{}, invalidf("invalid profile field length")
	}
	return profile, nil
}

func NormalizeBottomNavigation(items []string) ([]string, error) {
	if len(items) < 2 || len(items) > 6 {
		return nil, invalidf("bottom navigation must contain between two and six items")
	}
	allowed := map[string]bool{
		"card": true, "tasks": true, "projects": true,
		"tracker": true, "calories": true, "profile": true,
	}
	seen := make(map[string]bool, len(items))
	normalized := make([]string, len(items))
	for index, item := range items {
		item = strings.TrimSpace(item)
		if !allowed[item] {
			return nil, invalidf("unknown bottom navigation item %q", item)
		}
		if seen[item] {
			return nil, invalidf("bottom navigation contains duplicate item %q", item)
		}
		seen[item] = true
		normalized[index] = item
	}
	for _, required := range []string{"card", "profile"} {
		if !seen[required] {
			return nil, invalidf("bottom navigation must contain %q", required)
		}
	}
	return normalized, nil
}

const fixedCardExpiryDate = "13.07.2106"

func normalizeCompactProfileDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 8 {
		return value
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return value
		}
	}
	return value[:2] + "." + value[2:4] + "." + value[4:]
}

func invalidf(format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	return domain.InvalidInputError{Message: message}
}

var allowedTrackerIcons = func() map[string]bool {
	icons := map[string]bool{
		// Legacy identifiers remain valid so existing custom trackers are not broken.
		"target": true, "home": true, "work": true, "book": true, "study": true,
		"fitness": true, "run": true, "bike": true, "walk": true, "water": true,
		"food": true, "sleep": true, "medicine": true, "heart": true, "money": true,
		"save": true, "shopping": true, "travel": true, "car": true, "plant": true,
		"pet": true, "music": true, "art": true, "camera": true, "code": true,
		"language": true, "habit": true, "clean": true, "family": true, "star": true,
	}
	for index := 1; index <= 36; index++ {
		icons[fmt.Sprintf("icon-%02d", index)] = true
	}
	return icons
}()

func NormalizeCategory(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 40 {
		return "", invalidf("category name must be 1..40 characters")
	}
	return name, nil
}

func NormalizeCustomTracker(input domain.CustomTrackerInput) (domain.CustomTrackerInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Icon = strings.ToLower(strings.TrimSpace(input.Icon))
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 40 {
		return domain.CustomTrackerInput{}, invalidf("tracker name must be 1..40 characters")
	}
	if input.TargetValue <= 0 || input.TargetValue > 1_000_000_000 || math.IsNaN(input.TargetValue) || math.IsInf(input.TargetValue, 0) {
		return domain.CustomTrackerInput{}, invalidf("tracker targetValue is invalid")
	}
	if input.StepValue <= 0 || input.StepValue > input.TargetValue || math.IsNaN(input.StepValue) || math.IsInf(input.StepValue, 0) {
		return domain.CustomTrackerInput{}, invalidf("tracker stepValue is invalid")
	}
	if !allowedTrackerIcons[input.Icon] {
		return domain.CustomTrackerInput{}, invalidf("tracker icon is invalid")
	}
	input.TargetValue = math.Round(input.TargetValue*1000) / 1000
	input.StepValue = math.Round(input.StepValue*1000) / 1000
	return input, nil
}
