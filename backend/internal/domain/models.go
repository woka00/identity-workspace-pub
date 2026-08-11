package domain

import (
	"context"
	"errors"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInvalidInput = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
)

type InvalidInputError struct{ Message string }

func (e InvalidInputError) Error() string        { return e.Message }
func (e InvalidInputError) Is(target error) bool { return target == ErrInvalidInput }

type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	CreatedAt string `json:"createdAt"`
}

type UserCredential struct {
	User
	PasswordHash string `json:"-"`
}

type AuthSession struct {
	User      User   `json:"user"`
	Token     string `json:"-"`
	ExpiresAt string `json:"expiresAt"`
}

type userIDContextKey struct{}

func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

func UserID(ctx context.Context) (int64, error) {
	userID, ok := ctx.Value(userIDContextKey{}).(int64)
	if !ok || userID <= 0 {
		return 0, ErrUnauthorized
	}
	return userID, nil
}

type Profile struct {
	Name       string `json:"name"`
	Surname    string `json:"surname"`
	Occupation string `json:"occupation"`
	Sex        string `json:"sex"`
	DOB        string `json:"dob"`
	Expiry     string `json:"expiry"`
	Photo      string `json:"photo"`
	Signature  string `json:"signature"`
}

type Task struct {
	ID                 int64  `json:"id"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	Category           string `json:"category"`
	Status             string `json:"status"`
	DueDate            string `json:"dueDate"`
	DueTime            string `json:"dueTime"`
	ReminderAt         string `json:"reminderAt"`
	ReminderSentAt     string `json:"reminderSentAt"`
	Priority           int    `json:"priority"`
	CreatedAt          string `json:"createdAt"`
	CompletedAt        string `json:"completedAt"`
	IsMilestone        bool   `json:"isMilestone"`
	TickTickSyncStatus string `json:"tickTickSyncStatus"`
	TickTickSyncError  string `json:"tickTickSyncError,omitempty"`
}

type TaskInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	DueDate     string `json:"dueDate"`
	DueTime     string `json:"dueTime"`
	ReminderAt  string `json:"reminderAt"`
	Priority    int    `json:"priority"`
	IsMilestone bool   `json:"isMilestone"`
}

type Goal struct {
	ID             int64    `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Summary        string   `json:"summary"`
	CurrentValue   float64  `json:"currentValue"`
	TargetValue    float64  `json:"targetValue"`
	Unit           string   `json:"unit"`
	Deadline       string   `json:"deadline"`
	RelatedTaskIDs []string `json:"relatedTaskIds"`
	Completed      bool     `json:"completed"`
	CompletedAt    string   `json:"completedAt"`
	Pinned         bool     `json:"pinned"`
	SortOrder      int64    `json:"sortOrder"`
	CompletionPct  int      `json:"completionPct"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
}

type GoalInput struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Summary        string   `json:"summary"`
	CurrentValue   float64  `json:"currentValue"`
	TargetValue    float64  `json:"targetValue"`
	Unit           string   `json:"unit"`
	Deadline       string   `json:"deadline"`
	RelatedTaskIDs []string `json:"relatedTaskIds"`
	Completed      bool     `json:"completed"`
	Pinned         bool     `json:"pinned"`
}

type Portfolio struct {
	Pinned    []Goal `json:"pinned"`
	Completed []Goal `json:"completed"`
}

type TrackerWeightEntry struct {
	Date      string  `json:"date"`
	WeightKg  float64 `json:"weightKg"`
	UpdatedAt string  `json:"updatedAt"`
}

type TrackerWaterEntry struct {
	Date        string `json:"date"`
	Glasses     int    `json:"glasses"`
	GoalGlasses int    `json:"goalGlasses"`
	UpdatedAt   string `json:"updatedAt"`
}

type CustomTracker struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	TargetValue  float64 `json:"targetValue"`
	StepValue    float64 `json:"stepValue"`
	CurrentValue float64 `json:"currentValue"`
	Icon         string  `json:"icon"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

type CustomTrackerInput struct {
	Name        string  `json:"name"`
	TargetValue float64 `json:"targetValue"`
	StepValue   float64 `json:"stepValue"`
	Icon        string  `json:"icon"`
}

type CustomTrackerEntry struct {
	TrackerID   int64   `json:"trackerId"`
	Date        string  `json:"date"`
	Value       float64 `json:"value"`
	TargetValue float64 `json:"targetValue"`
	UpdatedAt   string  `json:"updatedAt"`
}

type TaskCategory struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
}

type PushSubscriptionInput struct {
	Endpoint string `json:"endpoint"`
	P256DH   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

type PushSubscription struct {
	ID       int64
	UserID   int64
	Endpoint string
	P256DH   string
	Auth     string
}

type ReminderJob struct {
	TaskID        int64
	UserID        int64
	Title         string
	Description   string
	DueDate       string
	DueTime       string
	ReminderAt    string
	Subscriptions []PushSubscription
}

type NotificationConfig struct {
	Configured bool   `json:"configured"`
	PublicKey  string `json:"publicKey"`
}

type TrackerReminder struct {
	TrackerKey string `json:"trackerKey"`
	Time       string `json:"time"`
	Enabled    bool   `json:"enabled"`
}

type TrackerReminderInput struct {
	TrackerKey string `json:"trackerKey"`
	Time       string `json:"time"`
	Enabled    bool   `json:"enabled"`
}

type TrackerReminderJob struct {
	UserID        int64
	TrackerKey    string
	Title         string
	Time          string
	LocalDate     string
	Subscriptions []PushSubscription
}

type TrackerState struct {
	WaterGoal       int                  `json:"waterGoal"`
	CalorieGoal     int                  `json:"calorieGoal"`
	CurrentWeightKg *float64             `json:"currentWeightKg"`
	WeightHistory   []TrackerWeightEntry `json:"weightHistory"`
	WaterHistory    []TrackerWaterEntry  `json:"waterHistory"`
	CustomTrackers  []CustomTracker      `json:"customTrackers"`
	CustomHistory   []CustomTrackerEntry `json:"customHistory"`
}

type FatSecretConnection struct {
	OAuthToken       string `json:"-"`
	OAuthTokenSecret string `json:"-"`
	ConnectedAt      string `json:"connectedAt"`
}

type FatSecretOAuthRequest struct {
	UserID           int64
	OAuthToken       string
	OAuthTokenSecret string
	ReturnTo         string
}

type TickTickConnection struct {
	AccessToken string `json:"-"`
	ProjectID   string `json:"-"`
	ProjectName string `json:"projectName"`
	ConnectedAt string `json:"connectedAt"`
}

type TickTickOAuthState struct {
	UserID      int64
	State       string
	CallbackURL string
	ReturnTo    string
}

type TickTickTaskLink struct {
	TaskID         int64
	TickTickTaskID string
	ProjectID      string
	SyncStatus     string
	LastError      string
}

type TickTickStatus struct {
	Configured   bool   `json:"configured"`
	Connected    bool   `json:"connected"`
	ConnectedAt  string `json:"connectedAt"`
	ProjectName  string `json:"projectName"`
	PendingTasks int    `json:"pendingTasks"`
	FailedTasks  int    `json:"failedTasks"`
}

type TickTickRemoteTask struct {
	ID          string
	ProjectID   string
	Title       string
	Description string
	DueDate     string
	DueTime     string
	Priority    int
	Status      string
	IsMilestone bool
}

type TickTickPullResult struct {
	Imported  int
	Updated   int
	Completed int
}

type TickTickSyncResult struct {
	Synced    int `json:"synced"`
	Failed    int `json:"failed"`
	Imported  int `json:"imported"`
	Updated   int `json:"updated"`
	Completed int `json:"completed"`
}

type Nutrition struct {
	Date         string          `json:"date"`
	Calories     float64         `json:"calories"`
	Carbohydrate float64         `json:"carbohydrate"`
	Protein      float64         `json:"protein"`
	Fat          float64         `json:"fat"`
	EntryCount   int             `json:"entryCount"`
	Meals        []MealNutrition `json:"meals"`
	FetchedAt    string          `json:"fetchedAt"`
}

type MealNutrition struct {
	Meal         string  `json:"meal"`
	Calories     float64 `json:"calories"`
	Carbohydrate float64 `json:"carbohydrate"`
	Protein      float64 `json:"protein"`
	Fat          float64 `json:"fat"`
	EntryCount   int     `json:"entryCount"`
}

type State struct {
	Profile     Profile `json:"profile"`
	ActiveTasks []Task  `json:"activeTasks"`
	CurrentDate string  `json:"currentDate"`
}

type FatSecretStatus struct {
	Configured  bool   `json:"configured"`
	Connected   bool   `json:"connected"`
	ConnectedAt string `json:"connectedAt"`
}
