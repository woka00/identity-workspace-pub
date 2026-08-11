package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"identity-workspace/internal/domain"
)

type Repository interface {
	UserByLogin(context.Context, string) (domain.UserCredential, error)
	UpdatePasswordHash(context.Context, int64, string) error
	CreateSession(context.Context, int64, string, time.Time) error
	UserBySession(context.Context, string, time.Time) (domain.User, error)
	DeleteSession(context.Context, string) error

	Profile(context.Context) (domain.Profile, error)
	UpdateProfile(context.Context, domain.Profile) error
	SetPhoto(context.Context, string) error
	SetSignature(context.Context, string) error

	Trackers(context.Context) (domain.TrackerState, error)
	UpsertTrackerWeight(context.Context, string, float64) (domain.TrackerWeightEntry, error)
	UpsertTrackerWater(context.Context, string, int, int) (domain.TrackerWaterEntry, error)
	UpdateCalorieGoal(context.Context, int) (int, error)
	CustomTrackers(context.Context) ([]domain.CustomTracker, error)
	CreateCustomTracker(context.Context, domain.CustomTrackerInput) (domain.CustomTracker, error)
	UpdateCustomTracker(context.Context, int64, domain.CustomTrackerInput) (domain.CustomTracker, error)
	StepCustomTracker(context.Context, int64, string, int) (domain.CustomTracker, error)
	DeleteCustomTracker(context.Context, int64) error

	TaskCategories(context.Context) ([]domain.TaskCategory, error)
	CreateTaskCategory(context.Context, string) (domain.TaskCategory, error)
	DeleteTaskCategory(context.Context, int64) error

	Tasks(context.Context) ([]domain.Task, error)
	ActiveTasks(context.Context) ([]domain.Task, error)
	CreateTask(context.Context, domain.TaskInput) (domain.Task, error)
	UpdateTask(context.Context, int64, domain.TaskInput) (domain.Task, error)
	SetTaskCompleted(context.Context, int64, bool) (domain.Task, error)
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

	TickTickConnection(context.Context) (domain.TickTickConnection, error)
	SaveTickTickOAuthState(context.Context, domain.TickTickOAuthState) error
	ConsumeTickTickOAuthState(context.Context, string) (domain.TickTickOAuthState, error)
	SaveTickTickConnection(context.Context, int64, string, string, string) error
	DeleteTickTickConnection(context.Context) error
	TickTickTaskLink(context.Context, int64) (domain.TickTickTaskLink, error)
	SaveTickTickTaskLink(context.Context, domain.TickTickTaskLink) error
	TickTickPendingTasks(context.Context) ([]domain.Task, error)
	ApplyTickTickSnapshot(context.Context, []domain.TickTickRemoteTask) (domain.TickTickPullResult, error)
	TickTickSyncCounts(context.Context) (int, int, error)

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
}

type Service struct {
	repo      Repository
	fatSecret FatSecretGateway
	tickTick  TickTickGateway
	push      PushGateway
	now       func() time.Time

	tickTickLocksMu sync.Mutex
	tickTickLocks   map[int64]*sync.Mutex
}

func New(repo Repository, fatSecret FatSecretGateway, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, fatSecret: fatSecret, now: now, tickTickLocks: make(map[int64]*sync.Mutex)}
}

func (s *Service) WithTickTick(gateway TickTickGateway) *Service {
	s.tickTick = gateway
	return s
}

func (s *Service) WithPush(gateway PushGateway) *Service {
	s.push = gateway
	return s
}

func (s *Service) Today() string { return s.now().Format("2006-01-02") }

func (s *Service) State(ctx context.Context) (domain.State, error) {
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
	if err := validateSignatureDataURL(profile.Signature); err != nil {
		profile.Signature = ""
	}
	tasks, err := s.repo.ActiveTasks(ctx)
	if err != nil {
		return domain.State{}, err
	}
	return domain.State{
		Profile: profile, ActiveTasks: tasks, CurrentDate: s.Today(),
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
	return s.repo.Trackers(ctx)
}

func (s *Service) UpdateCalorieGoal(ctx context.Context, calorieGoal int) (int, error) {
	if err := ValidateCalorieGoal(calorieGoal); err != nil {
		return 0, err
	}
	return s.repo.UpdateCalorieGoal(ctx, calorieGoal)
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
	return s.syncTickTickTaskBestEffort(ctx, task, true), nil
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
	return s.syncTickTickTaskBestEffort(ctx, task, false), nil
}

func (s *Service) SetTaskCompleted(ctx context.Context, id int64, completed bool) (domain.Task, error) {
	if id <= 0 {
		return domain.Task{}, invalidf("invalid task id")
	}
	task, err := s.repo.SetTaskCompleted(ctx, id, completed)
	if err != nil {
		return domain.Task{}, err
	}
	return s.syncTickTickTaskBestEffort(ctx, task, false), nil
}

func (s *Service) DeleteTask(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidf("invalid task id")
	}
	unlock := s.lockTickTick(ctx)
	defer unlock()
	var connection domain.TickTickConnection
	var link domain.TickTickTaskLink
	connection, connectionErr := s.repo.TickTickConnection(ctx)
	link, linkErr := s.repo.TickTickTaskLink(ctx, id)
	if err := s.repo.DeleteTask(ctx, id); err != nil {
		return err
	}
	if s.tickTick != nil && connectionErr == nil && linkErr == nil && link.TickTickTaskID != "" {
		_ = s.tickTick.DeleteTask(ctx, connection.AccessToken, link.ProjectID, link.TickTickTaskID)
	}
	return nil
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
