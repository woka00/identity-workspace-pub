package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"avatar-id/internal/domain"
)

type Repository interface {
	Ping(context.Context) error
	UserByLogin(context.Context, string) (domain.UserCredential, error)
	UpdatePasswordHash(context.Context, int64, string) error
	CreateSession(context.Context, int64, string, time.Time) error
	UserBySession(context.Context, string, time.Time) (domain.User, error)
	DeleteSession(context.Context, string) error

	Profile(context.Context) (domain.Profile, error)
	UpdateProfile(context.Context, domain.Profile) error
	UpdateWorkProfile(context.Context, domain.WorkProfileInput) error
	SetPhoto(context.Context, string) error
	SetSignature(context.Context, string) error
	BottomNavigation(context.Context) ([]string, error)
	UpdateBottomNavigation(context.Context, []string) error
	WorkspacePreferences(context.Context) (domain.WorkspacePreferences, error)
	UpdateWorkspacePreferences(context.Context, domain.WorkspacePreferences) error

	Trackers(context.Context) (domain.TrackerState, error)
	UpsertTrackerWeight(context.Context, string, float64) (domain.TrackerWeightEntry, error)
	UpsertTrackerWater(context.Context, string, int, int) (domain.TrackerWaterEntry, error)
	UpdateNutritionGoals(context.Context, domain.NutritionGoals) (domain.NutritionGoals, error)
	CustomTrackers(context.Context) ([]domain.CustomTracker, error)
	CreateCustomTracker(context.Context, domain.CustomTrackerInput) (domain.CustomTracker, error)
	UpdateCustomTracker(context.Context, int64, domain.CustomTrackerInput) (domain.CustomTracker, error)
	StepCustomTracker(context.Context, int64, string, int) (domain.CustomTracker, error)
	DeleteCustomTracker(context.Context, int64) error
	GitHubTracker(context.Context) (domain.GitHubTrackerRecord, error)
	UpsertGitHubTracker(context.Context, string) error
	CacheGitHubTracker(context.Context, string, domain.GitHubTrackerState) error
	DeleteGitHubTracker(context.Context) error

	TaskCategories(context.Context) ([]domain.TaskCategory, error)
	CreateTaskCategory(context.Context, string) (domain.TaskCategory, error)
	DeleteTaskCategory(context.Context, int64) error

	Tasks(context.Context) ([]domain.Task, error)
	ActiveTasks(context.Context) ([]domain.Task, error)
	CreateTask(context.Context, domain.TaskInput) (domain.Task, error)
	CreateRecurringTask(context.Context, int64, domain.TaskInput) (domain.Task, bool, error)
	UpdateTask(context.Context, int64, domain.TaskInput) (domain.Task, error)
	SetTaskCompleted(context.Context, int64, bool) (domain.Task, error)
	SwapTaskOrder(context.Context, int64, int64) error
	DeleteTask(context.Context, int64) error

	Goal(context.Context, int64) (domain.Goal, error)
	Goals(context.Context) ([]domain.Goal, error)
	Portfolio(context.Context) (domain.Portfolio, error)
	CreateGoal(context.Context, domain.GoalInput) (domain.Goal, error)
	UpdateGoal(context.Context, int64, domain.GoalInput) (domain.Goal, error)
	DeleteGoal(context.Context, int64) error
	ReorderGoals(context.Context, []int64) error

	FatSecretConnection(context.Context) (domain.FatSecretConnection, error)
	SaveFatSecretOAuthRequest(context.Context, domain.FatSecretOAuthRequest) error
	ConsumeFatSecretOAuthRequest(context.Context, string) (domain.FatSecretOAuthRequest, error)
	SaveFatSecretConnection(context.Context, int64, string, string) error
	DeleteFatSecretConnection(context.Context) error

	SearchFoodCatalog(context.Context, string, int) ([]domain.FoodCatalogItem, bool, error)
	RecentFoodCatalog(context.Context, string) ([]domain.FoodCatalogItem, error)
	FoodCatalogItem(context.Context, string) (domain.FoodCatalogItem, error)
	FoodCatalogByBarcode(context.Context, string) (domain.FoodCatalogItem, error)
	FoodCatalogCandidates(context.Context, domain.FoodCatalogCandidateInput) ([]domain.FoodCatalogItem, error)
	LinkFoodCatalogBarcode(context.Context, string, string) (domain.FoodCatalogItem, error)
	DeleteFoodCatalog(context.Context, string) error
	AdminFoodCatalog(context.Context, int, bool) (domain.FoodCatalogReviewPage, error)
	UpdateAdminFoodCatalog(context.Context, int64, domain.FoodCatalogReviewInput) (domain.FoodCatalogReviewItem, error)
	PromoteFoodCatalogItem(context.Context, int64) (domain.FoodCatalogItem, error)
	RejectFoodCatalogItem(context.Context, int64) error
	UpsertFoodCatalog(context.Context, domain.FoodCatalogItem) (domain.FoodCatalogItem, error)
	Nutrition(context.Context, string) (domain.Nutrition, error)
	CreateNutritionEntry(context.Context, domain.LocalNutritionEntryInput) (domain.Nutrition, error)
	UpdateNutritionEntry(context.Context, int64, domain.LocalNutritionEntryInput) (domain.Nutrition, error)
	DeleteNutritionEntry(context.Context, int64, string) (domain.Nutrition, error)

	SavePushSubscription(context.Context, domain.PushSubscriptionInput, string) error
	DeletePushSubscription(context.Context, string) error
	ClaimDueReminders(context.Context, time.Time, int) ([]domain.ReminderJob, error)
	CompleteReminder(context.Context, int64, bool) error
	DeletePushSubscriptionByID(context.Context, int64) error

	Reset(context.Context) error
}

type FatSecretGateway interface {
	Configured() bool
	AuthorizeURL(token string) string
	RequestToken(context.Context, string) (string, string, error)
	AccessToken(context.Context, string, string, string) (string, string, error)
	Nutrition(context.Context, string, string, string) (domain.Nutrition, error)
	SearchFoods(context.Context, string, string, string, int) (domain.FoodSearchPage, error)
	Food(context.Context, string, string, string) (domain.Food, error)
	BarcodeFood(context.Context, string) (domain.Food, error)
	RecentFoods(context.Context, string, string, string) ([]domain.Food, error)
	CreateFoodEntry(context.Context, string, string, domain.FoodEntryInput) error
	UpdateFoodEntry(context.Context, string, string, string, domain.FoodEntryUpdate) error
	DeleteFoodEntry(context.Context, string, string, string) error
}

type FoodCatalogGateway interface {
	Search(context.Context, string, int) ([]domain.FoodCatalogItem, bool, error)
	Barcode(context.Context, string) (domain.FoodCatalogItem, error)
}

type Service struct {
	registration *Registration
	repo         Repository
	fatSecret    FatSecretGateway
	push         PushGateway
	github       GitHubGateway
	foodCatalog  FoodCatalogGateway
	now          func() time.Time
}

func (s *Service) WithFoodCatalog(gateway FoodCatalogGateway) *Service {
	s.foodCatalog = gateway
	return s
}

func New(repo Repository, fatSecret FatSecretGateway, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, fatSecret: fatSecret, now: now}
}

func (s *Service) WithPush(gateway PushGateway) *Service {
	s.push = gateway
	return s
}

func (s *Service) WithGitHub(gateway GitHubGateway) *Service {
	s.github = gateway
	return s
}

func (s *Service) Today() string { return s.now().Format("2006-01-02") }

func (s *Service) State(ctx context.Context) (domain.State, error) {
	return s.state(ctx, true)
}

func (s *Service) StateWithoutActiveTasks(ctx context.Context) (domain.State, error) {
	return s.state(ctx, false)
}

func (s *Service) Ready(ctx context.Context) error { return s.repo.Ping(ctx) }

func (s *Service) state(ctx context.Context, includeActiveTasks bool) (domain.State, error) {
	profile, err := s.repo.Profile(ctx)
	if err != nil {
		return domain.State{}, err
	}
	// Older databases may contain a photo saved before strict file validation
	// was introduced. Never send unsupported or malformed active content back
	// to the browser.
	if err := validatePhotoDataURL(profile.Photo); err != nil {
		profile.Photo = ""
	}
	if profile.WorkAvatar != "" {
		if err := validatePhotoDataURL(profile.WorkAvatar); err != nil {
			profile.WorkAvatar = ""
		}
	}
	if err := validateSignatureDataURL(profile.Signature); err != nil {
		profile.Signature = ""
	}
	profile.Expiry = fixedCardExpiryDate
	tasks := []domain.Task{}
	if includeActiveTasks {
		tasks, err = s.repo.ActiveTasks(ctx)
		if err != nil {
			return domain.State{}, err
		}
	}
	bottomNavigation, err := s.repo.BottomNavigation(ctx)
	if err != nil {
		return domain.State{}, err
	}
	preferences, err := s.repo.WorkspacePreferences(ctx)
	if err != nil {
		return domain.State{}, err
	}
	return domain.State{
		Profile: profile, ActiveTasks: tasks, CurrentDate: s.Today(), BottomNavigation: bottomNavigation, WorkspacePreferences: preferences,
	}, nil
}

func (s *Service) FatSecretStatus(ctx context.Context) (domain.FatSecretStatus, error) {
	status := domain.FatSecretStatus{Configured: s.fatSecret != nil && s.fatSecret.Configured()}
	connection, err := s.repo.FatSecretConnection(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return status, nil
	}
	if err != nil {
		return domain.FatSecretStatus{}, err
	}
	status.Connected = true
	status.ConnectedAt = connection.ConnectedAt
	return status, nil
}

func (s *Service) BeginFatSecretConnection(ctx context.Context, callbackURL, returnTo string) (string, error) {
	if s.fatSecret == nil || !s.fatSecret.Configured() {
		return "", fmt.Errorf("FatSecret is not configured: %w", domain.ErrConflict)
	}
	token, secret, err := s.fatSecret.RequestToken(ctx, callbackURL)
	if err != nil {
		return "", err
	}
	if err := s.repo.SaveFatSecretOAuthRequest(ctx, domain.FatSecretOAuthRequest{
		OAuthToken: token, OAuthTokenSecret: secret, ReturnTo: returnTo,
	}); err != nil {
		return "", err
	}
	return s.fatSecret.AuthorizeURL(token), nil
}

func (s *Service) CompleteFatSecretConnection(ctx context.Context, token, verifier string) (string, error) {
	if s.fatSecret == nil || !s.fatSecret.Configured() {
		return "", fmt.Errorf("FatSecret is not configured: %w", domain.ErrConflict)
	}
	if token == "" || verifier == "" {
		return "", fmt.Errorf("FatSecret authorization was denied: %w", domain.ErrConflict)
	}
	pending, err := s.repo.ConsumeFatSecretOAuthRequest(ctx, token)
	if err != nil {
		return "", err
	}
	accessToken, accessSecret, err := s.fatSecret.AccessToken(ctx, token, pending.OAuthTokenSecret, verifier)
	if err != nil {
		return pending.ReturnTo, err
	}
	if err := s.repo.SaveFatSecretConnection(ctx, pending.UserID, accessToken, accessSecret); err != nil {
		return pending.ReturnTo, err
	}
	return pending.ReturnTo, nil
}

func (s *Service) DisconnectFatSecret(ctx context.Context) error {
	return s.repo.DeleteFatSecretConnection(ctx)
}

func (s *Service) Nutrition(ctx context.Context, date string) (domain.Nutrition, error) {
	if s.fatSecret == nil || !s.fatSecret.Configured() {
		return domain.Nutrition{}, fmt.Errorf("FatSecret is not configured: %w", domain.ErrConflict)
	}
	date, err := NormalizeDate(date, "nutrition date")
	if err != nil {
		return domain.Nutrition{}, err
	}
	connection, err := s.repo.FatSecretConnection(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Nutrition{}, fmt.Errorf("FatSecret account is not connected: %w", domain.ErrConflict)
	}
	if err != nil {
		return domain.Nutrition{}, err
	}
	return s.fatSecret.Nutrition(ctx, connection.OAuthToken, connection.OAuthTokenSecret, date)
}

func (s *Service) Trackers(ctx context.Context) (domain.TrackerState, error) {
	state, err := s.repo.Trackers(ctx)
	if err != nil {
		return domain.TrackerState{}, err
	}
	return customTrackerStateForDate(state, s.Today()), nil
}

func customTrackerStateForDate(state domain.TrackerState, date string) domain.TrackerState {
	values := make(map[int64]domain.CustomTrackerEntry, len(state.CustomHistory))
	for _, entry := range state.CustomHistory {
		if entry.Date == date {
			values[entry.TrackerID] = entry
		}
	}
	for index := range state.CustomTrackers {
		tracker := &state.CustomTrackers[index]
		tracker.CurrentValue = 0
		if entry, ok := values[tracker.ID]; ok {
			tracker.CurrentValue = entry.Value
			tracker.UpdatedAt = entry.UpdatedAt
		}
	}
	return state
}

func (s *Service) UpdateNutritionGoals(ctx context.Context, goals domain.NutritionGoals) (domain.NutritionGoals, error) {
	if err := ValidateNutritionGoals(goals); err != nil {
		return domain.NutritionGoals{}, err
	}
	return s.repo.UpdateNutritionGoals(ctx, goals)
}

func (s *Service) UpsertWeight(ctx context.Context, date string, weightKg float64) (domain.TrackerWeightEntry, error) {
	date, err := NormalizeDate(date, "tracker date")
	if err != nil {
		return domain.TrackerWeightEntry{}, err
	}
	weightKg, err = NormalizeWeight(weightKg)
	if err != nil {
		return domain.TrackerWeightEntry{}, err
	}
	return s.repo.UpsertTrackerWeight(ctx, date, weightKg)
}

func (s *Service) UpsertWater(ctx context.Context, date string, glasses, goalGlasses int) (domain.TrackerWaterEntry, error) {
	date, err := NormalizeDate(date, "tracker date")
	if err != nil {
		return domain.TrackerWaterEntry{}, err
	}
	if err := ValidateWater(glasses, goalGlasses); err != nil {
		return domain.TrackerWaterEntry{}, err
	}
	return s.repo.UpsertTrackerWater(ctx, date, glasses, goalGlasses)
}

func (s *Service) Tasks(ctx context.Context) ([]domain.Task, error) { return s.repo.Tasks(ctx) }

func (s *Service) CreateTask(ctx context.Context, input domain.TaskInput) (domain.Task, error) {
	input, err := NormalizeTask(input, true)
	if err != nil {
		return domain.Task{}, err
	}
	task, err := s.repo.CreateTask(ctx, input)
	if err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *Service) UpdateTask(ctx context.Context, id int64, input domain.TaskInput) (domain.Task, error) {
	if id <= 0 {
		return domain.Task{}, invalidf("invalid task id")
	}
	input, err := NormalizeTask(input, false)
	if err != nil {
		return domain.Task{}, err
	}
	task, err := s.repo.UpdateTask(ctx, id, input)
	if err != nil {
		return domain.Task{}, err
	}
	if task.Status == "done" {
		if err := s.createNextRecurringTask(ctx, task); err != nil {
			return domain.Task{}, err
		}
	}
	return task, nil
}

func (s *Service) SetTaskCompleted(ctx context.Context, id int64, completed bool) (domain.Task, error) {
	if id <= 0 {
		return domain.Task{}, invalidf("invalid task id")
	}
	task, err := s.repo.SetTaskCompleted(ctx, id, completed)
	if err != nil {
		return domain.Task{}, err
	}
	if !completed {
		return task, nil
	}
	if err := s.createNextRecurringTask(ctx, task); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *Service) SwapTaskOrder(ctx context.Context, id, otherID int64) ([]domain.Task, error) {
	if id <= 0 || otherID <= 0 || id == otherID {
		return nil, invalidf("invalid task order")
	}
	if err := s.repo.SwapTaskOrder(ctx, id, otherID); err != nil {
		return nil, err
	}
	return s.repo.Tasks(ctx)
}

func (s *Service) createNextRecurringTask(ctx context.Context, task domain.Task) error {
	if task.RecurrenceType == "" {
		return nil
	}
	nextInput, ok := nextRecurringTaskInput(task)
	if !ok {
		return nil
	}
	_, _, err := s.repo.CreateRecurringTask(ctx, task.ID, nextInput)
	return err
}

func nextRecurringTaskInput(task domain.Task) (domain.TaskInput, bool) {
	dueDate, err := time.Parse("2006-01-02", task.DueDate)
	if err != nil || task.RecurrenceInterval < 1 {
		return domain.TaskInput{}, false
	}
	var nextDate time.Time
	switch task.RecurrenceType {
	case "daily":
		nextDate = dueDate.AddDate(0, 0, task.RecurrenceInterval)
	case "weekly":
		nextDate = nextWeeklyRecurrenceDate(dueDate, task.RecurrenceInterval, task.RecurrenceWeekdays)
	case "monthly":
		nextDate = addMonthsClamped(dueDate, task.RecurrenceInterval)
	default:
		return domain.TaskInput{}, false
	}
	nextDateKey := nextDate.Format("2006-01-02")
	if task.RecurrenceEndDate != "" && nextDateKey > task.RecurrenceEndDate {
		return domain.TaskInput{}, false
	}
	reminderAt := ""
	if reminder, reminderErr := time.Parse(time.RFC3339, task.ReminderAt); reminderErr == nil {
		years, months, days := dateDifference(dueDate, nextDate)
		reminderAt = reminder.AddDate(years, months, days).UTC().Format(time.RFC3339)
	}
	return domain.TaskInput{
		Title: task.Title, Description: task.Description, Category: task.Category,
		Status: "todo", DueDate: nextDateKey, DueTime: task.DueTime,
		ReminderAt: reminderAt, Priority: task.Priority, IsMilestone: task.IsMilestone,
		RecurrenceType: task.RecurrenceType, RecurrenceInterval: task.RecurrenceInterval,
		RecurrenceEndDate: task.RecurrenceEndDate, RecurrenceWeekdays: task.RecurrenceWeekdays,
	}, true
}

func nextWeeklyRecurrenceDate(date time.Time, interval int, weekdays []int) time.Time {
	currentWeekday := int(date.Weekday())
	if currentWeekday == 0 {
		currentWeekday = 7
	}
	for _, weekday := range weekdays {
		if weekday > currentWeekday {
			return date.AddDate(0, 0, weekday-currentWeekday)
		}
	}
	firstWeekday := currentWeekday
	if len(weekdays) > 0 {
		firstWeekday = weekdays[0]
	}
	daysUntilNextCycle := (7 - currentWeekday) + firstWeekday + (interval-1)*7
	return date.AddDate(0, 0, daysUntilNextCycle)
}

func addMonthsClamped(date time.Time, months int) time.Time {
	firstOfTarget := time.Date(date.Year(), date.Month()+time.Month(months), 1, 0, 0, 0, 0, date.Location())
	lastDay := firstOfTarget.AddDate(0, 1, -1).Day()
	day := date.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(firstOfTarget.Year(), firstOfTarget.Month(), day, 0, 0, 0, 0, date.Location())
}

func dateDifference(from, to time.Time) (years, months, days int) {
	if from.Day() == to.Day() {
		return to.Year() - from.Year(), int(to.Month() - from.Month()), 0
	}
	return 0, 0, int(to.Sub(from).Hours() / 24)
}

func (s *Service) DeleteTask(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid task id")
	}
	return s.repo.DeleteTask(ctx, id)
}

func (s *Service) Goal(ctx context.Context, id int64) (domain.Goal, error) {
	if id <= 0 {
		return domain.Goal{}, invalidf("invalid project id")
	}
	return s.repo.Goal(ctx, id)
}

func (s *Service) Goals(ctx context.Context) ([]domain.Goal, error) { return s.repo.Goals(ctx) }
func (s *Service) Portfolio(ctx context.Context) (domain.Portfolio, error) {
	return s.repo.Portfolio(ctx)
}

func (s *Service) CreateGoal(ctx context.Context, input domain.GoalInput) (domain.Goal, error) {
	input, err := NormalizeGoal(input)
	if err != nil {
		return domain.Goal{}, err
	}
	return s.repo.CreateGoal(ctx, input)
}

func (s *Service) UpdateGoal(ctx context.Context, id int64, input domain.GoalInput) (domain.Goal, error) {
	if id <= 0 {
		return domain.Goal{}, invalidf("invalid project id")
	}
	input, err := NormalizeGoal(input)
	if err != nil {
		return domain.Goal{}, err
	}
	return s.repo.UpdateGoal(ctx, id, input)
}

func (s *Service) DeleteGoal(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid project id")
	}
	return s.repo.DeleteGoal(ctx, id)
}

func (s *Service) ReorderGoals(ctx context.Context, ids []int64) ([]domain.Goal, error) {
	if len(ids) == 0 {
		return nil, invalidf("project order is empty")
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, invalidf("invalid project id")
		}
		if _, exists := seen[id]; exists {
			return nil, invalidf("project order contains duplicates")
		}
		seen[id] = struct{}{}
	}
	if err := s.repo.ReorderGoals(ctx, ids); err != nil {
		return nil, err
	}
	return s.repo.Goals(ctx)
}

func (s *Service) UpdateProfile(ctx context.Context, profile domain.Profile) error {
	profile, err := NormalizeProfile(profile)
	if err != nil {
		return err
	}
	return s.repo.UpdateProfile(ctx, profile)
}

func (s *Service) UpdateWorkProfile(ctx context.Context, input domain.WorkProfileInput) error {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if len([]rune(input.DisplayName)) > 80 {
		return invalidf("ник должен содержать не более 80 символов")
	}
	if input.Avatar != "" {
		if err := validatePhotoDataURL(input.Avatar); err != nil {
			return err
		}
	}
	return s.repo.UpdateWorkProfile(ctx, input)
}

func (s *Service) UpdateBottomNavigation(ctx context.Context, items []string) error {
	items, err := NormalizeBottomNavigation(items)
	if err != nil {
		return err
	}
	return s.repo.UpdateBottomNavigation(ctx, items)
}

func (s *Service) UpdatePhoto(ctx context.Context, dataURL string) error {
	if err := validatePhotoDataURL(dataURL); err != nil {
		return err
	}
	return s.repo.SetPhoto(ctx, dataURL)
}

func (s *Service) UpdateSignature(ctx context.Context, dataURL string) error {
	if err := validateSignatureDataURL(dataURL); err != nil {
		return err
	}
	return s.repo.SetSignature(ctx, dataURL)
}

func (s *Service) Reset(ctx context.Context) error { return s.repo.Reset(ctx) }
