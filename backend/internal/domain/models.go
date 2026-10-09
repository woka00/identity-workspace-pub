package domain

import (
	"errors"
	"time"
)

const (
	AuthSessionIdleTimeout     = 7 * 24 * time.Hour
	AuthSessionAbsoluteTimeout = 30 * 24 * time.Hour
	AuthSessionTouchInterval   = 10 * time.Minute
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInvalidInput = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

type InvalidInputError struct{ Message string }

func (e InvalidInputError) Error() string        { return e.Message }
func (e InvalidInputError) Is(target error) bool { return target == ErrInvalidInput }

type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	CreatedAt string `json:"createdAt"`
	IsAdmin   bool   `json:"isAdmin"`
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

type Profile struct {
	Name            string `json:"name"`
	Surname         string `json:"surname"`
	Occupation      string `json:"occupation"`
	Sex             string `json:"sex"`
	DOB             string `json:"dob"`
	Expiry          string `json:"expiry"`
	Photo           string `json:"photo"`
	Signature       string `json:"signature"`
	WorkDisplayName string `json:"workDisplayName"`
	WorkAvatar      string `json:"workAvatar"`
	WorkShowAvatar  bool   `json:"workShowAvatar"`
}

type WorkProfileInput struct {
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
	ShowAvatar  bool   `json:"showAvatar"`
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
	RecurrenceType     string `json:"recurrenceType"`
	RecurrenceInterval int    `json:"recurrenceInterval"`
	RecurrenceEndDate  string `json:"recurrenceEndDate"`
	RecurrenceWeekdays []int  `json:"recurrenceWeekdays"`
	SortOrder          int64  `json:"sortOrder"`
}

type TaskInput struct {
	Title              string  `json:"title"`
	Description        string  `json:"description"`
	Category           string  `json:"category"`
	Status             string  `json:"status"`
	DueDate            string  `json:"dueDate"`
	DueTime            string  `json:"dueTime"`
	ReminderAt         string  `json:"reminderAt"`
	Priority           int     `json:"priority"`
	IsMilestone        bool    `json:"isMilestone"`
	RecurrenceType     string  `json:"recurrenceType"`
	RecurrenceInterval int     `json:"recurrenceInterval"`
	RecurrenceEndDate  string  `json:"recurrenceEndDate"`
	RecurrenceWeekdays []int   `json:"recurrenceWeekdays"`
	ProjectIDs         []int64 `json:"projectIds"`
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

type WorkMember struct {
	UserID int64  `json:"userId"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Role   string `json:"role"`
}

type WorkProject struct {
	ID              int64        `json:"id"`
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	Summary         string       `json:"summary"`
	Status          string       `json:"status"`
	Deadline        string       `json:"deadline"`
	Completed       bool         `json:"completed"`
	CompletedAt     string       `json:"completedAt"`
	Pinned          bool         `json:"pinned"`
	OwnerID         int64        `json:"ownerId"`
	Role            string       `json:"role"`
	TaskCount       int          `json:"taskCount"`
	CompletedCount  int          `json:"completedCount"`
	CompletionPct   int          `json:"completionPct"`
	ContentRevision int64        `json:"contentRevision"`
	Version         int64        `json:"version"`
	CreatedAt       string       `json:"createdAt"`
	UpdatedAt       string       `json:"updatedAt"`
	Members         []WorkMember `json:"members"`
}

type WorkSection struct {
	ID                   int64  `json:"id"`
	ProjectID            int64  `json:"projectId"`
	Title                string `json:"title"`
	SortOrder            int64  `json:"sortOrder"`
	CompletedSectionID   *int64 `json:"completedSectionId"`
	ResetCompletedOnMove bool   `json:"resetCompletedOnMove"`
	MoveCompletedToEnd   bool   `json:"moveCompletedToEnd"`
}

type WorkSectionInput struct {
	Title      string `json:"title"`
	RelativeTo *int64 `json:"relativeTo"`
	Placement  string `json:"placement"`
}

type WorkSectionCompletionInput struct {
	SectionID            *int64 `json:"sectionId"`
	ResetCompletedOnMove bool   `json:"resetCompletedOnMove"`
	MoveCompletedToEnd   bool   `json:"moveCompletedToEnd"`
}

type WorkTask struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"projectId"`
	SectionID   *int64 `json:"sectionId"`
	ParentID    *int64 `json:"parentId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CreatedByID *int64 `json:"createdById"`
	AssigneeID  *int64 `json:"assigneeId"`
	Assignee    string `json:"assignee"`
	DueDate     string `json:"dueDate"`
	Completed   bool   `json:"completed"`
	SortOrder   int64  `json:"sortOrder"`
	Version     int64  `json:"version"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type WorkComment struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"taskId"`
	AuthorID  int64  `json:"authorId"`
	Author    string `json:"author"`
	Avatar    string `json:"avatar"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

type WorkAttachment struct {
	ID        int64  `json:"id"`
	TaskID    *int64 `json:"taskId"`
	ProjectID int64  `json:"projectId"`
	Name      string `json:"name"`
	MimeType  string `json:"mimeType"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
}

type WorkAttachmentData struct {
	WorkAttachment
	Content []byte `json:"-"`
}

type WorkLink struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
}

type WorkLinkInput struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type WorkNote struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Version   int64  `json:"version"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type WorkNoteSummary struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	Preview   string `json:"preview"`
	Version   int64  `json:"version"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type WorkNoteInput struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Version int64  `json:"version"`
}

type WorkEvent struct {
	ID          int64  `json:"id"`
	ActorID     *int64 `json:"actorId"`
	Actor       string `json:"actor"`
	ActorAvatar string `json:"actorAvatar"`
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	CreatedAt   string `json:"createdAt"`
}

type WorkProjectDetail struct {
	Project     WorkProject      `json:"project"`
	Sections    []WorkSection    `json:"sections"`
	Tasks       []WorkTask       `json:"tasks"`
	Comments    []WorkComment    `json:"comments"`
	Attachments []WorkAttachment `json:"attachments"`
	Links       []WorkLink       `json:"links"`
	Notes       []WorkNote       `json:"notes"`
	Events      []WorkEvent      `json:"events"`
}

type WorkProjectRevision struct {
	Revision int64 `json:"revision"`
}

type WorkProjectResources struct {
	Revision    int64            `json:"revision"`
	Comments    []WorkComment    `json:"comments"`
	Attachments []WorkAttachment `json:"attachments"`
	Links       []WorkLink       `json:"links"`
	Notes       []WorkNote       `json:"notes"`
	Events      []WorkEvent      `json:"events"`
}

type WorkProjectResourceSummary struct {
	Revision    int64             `json:"revision"`
	Attachments []WorkAttachment  `json:"attachments"`
	Links       []WorkLink        `json:"links"`
	Notes       []WorkNoteSummary `json:"notes"`
	Events      []WorkEvent       `json:"events"`
}

type WorkProjectInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	Deadline    string `json:"deadline"`
	Completed   bool   `json:"completed"`
	Pinned      bool   `json:"pinned"`
	Version     int64  `json:"version"`
}

type WorkTaskInput struct {
	SectionID   *int64 `json:"sectionId"`
	ParentID    *int64 `json:"parentId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	AssigneeID  *int64 `json:"assigneeId"`
	DueDate     string `json:"dueDate"`
	Completed   bool   `json:"completed"`
	SortOrder   int64  `json:"sortOrder"`
	Version     int64  `json:"version"`
}

type WorkInvite struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

type Portfolio struct {
	Pinned    []Goal `json:"pinned"`
	Active    []Goal `json:"active"`
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
	WaterGoal        int                  `json:"waterGoal"`
	CalorieGoal      int                  `json:"calorieGoal"`
	ProteinGoal      int                  `json:"proteinGoal"`
	FatGoal          int                  `json:"fatGoal"`
	CarbohydrateGoal int                  `json:"carbohydrateGoal"`
	CurrentWeightKg  *float64             `json:"currentWeightKg"`
	WeightHistory    []TrackerWeightEntry `json:"weightHistory"`
	WaterHistory     []TrackerWaterEntry  `json:"waterHistory"`
	CustomTrackers   []CustomTracker      `json:"customTrackers"`
	CustomHistory    []CustomTrackerEntry `json:"customHistory"`
}

type NutritionGoals struct {
	CalorieGoal      int `json:"calorieGoal"`
	ProteinGoal      int `json:"proteinGoal"`
	FatGoal          int `json:"fatGoal"`
	CarbohydrateGoal int `json:"carbohydrateGoal"`
}

type TimeActivityInput struct {
	Name                    string `json:"name"`
	ReminderIntervalMinutes int    `json:"reminderIntervalMinutes"`
	ReminderMessage         string `json:"reminderMessage"`
}

type TimeActivity struct {
	ID                      int64  `json:"id"`
	Name                    string `json:"name"`
	TodaySeconds            int64  `json:"todaySeconds"`
	PeriodSeconds           int64  `json:"periodSeconds"`
	LastStartedDate         string `json:"lastStartedDate,omitempty"`
	Active                  bool   `json:"active"`
	SortOrder               int    `json:"sortOrder"`
	CreatedAt               string `json:"createdAt"`
	ReminderIntervalMinutes int    `json:"reminderIntervalMinutes"`
	ReminderMessage         string `json:"reminderMessage"`
}

type TimeActivityReminderJob struct {
	SessionID     int64
	ActivityID    int64
	ActivityName  string
	Message       string
	Subscriptions []PushSubscription
}

type ActiveTimeSession struct {
	ID             int64  `json:"id"`
	ActivityID     int64  `json:"activityId"`
	ActivityName   string `json:"activityName"`
	StartedAt      string `json:"startedAt"`
	ElapsedSeconds int64  `json:"elapsedSeconds"`
}

type TimeTrackerState struct {
	Date               string             `json:"date"`
	Period             string             `json:"period"`
	Activities         []TimeActivity     `json:"activities"`
	ActiveSession      *ActiveTimeSession `json:"activeSession"`
	PausedSession      *ActiveTimeSession `json:"pausedSession"`
	TodayTotalSeconds  int64              `json:"todayTotalSeconds"`
	PeriodTotalSeconds int64              `json:"periodTotalSeconds"`
}

type TimeTrackerRange struct {
	Date          string
	Period        string
	TodayStart    time.Time
	TomorrowStart time.Time
	PeriodStart   time.Time
	PeriodEnd     time.Time
	Now           time.Time
}

type TimeStatisticsActivity struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	TotalSeconds int64  `json:"totalSeconds"`
	SessionCount int    `json:"sessionCount"`
	SharePercent int    `json:"sharePercent"`
}

type TimeStatisticsDay struct {
	Date         string `json:"date"`
	TotalSeconds int64  `json:"totalSeconds"`
}

type TimeStatisticsSession struct {
	ID              int64  `json:"id"`
	ActivityID      int64  `json:"activityId"`
	ActivityName    string `json:"activityName"`
	Date            string `json:"date"`
	StartedAt       string `json:"startedAt"`
	EndedAt         string `json:"endedAt,omitempty"`
	DurationSeconds int64  `json:"durationSeconds"`
	Active          bool   `json:"active"`
}

type TimeStatistics struct {
	Date                  string                   `json:"date"`
	Period                string                   `json:"period"`
	PeriodStart           string                   `json:"periodStart"`
	PeriodEnd             string                   `json:"periodEnd"`
	TotalSeconds          int64                    `json:"totalSeconds"`
	ActivityCount         int                      `json:"activityCount"`
	SessionCount          int                      `json:"sessionCount"`
	LongestSessionSeconds int64                    `json:"longestSessionSeconds"`
	ActiveDays            int                      `json:"activeDays"`
	Activities            []TimeStatisticsActivity `json:"activities"`
	Days                  []TimeStatisticsDay      `json:"days"`
	Sessions              []TimeStatisticsSession  `json:"sessions"`
}

type TimeActivityStatistics struct {
	ActivityID            int64                   `json:"activityId"`
	Name                  string                  `json:"name"`
	TotalSeconds          int64                   `json:"totalSeconds"`
	WeekSeconds           int64                   `json:"weekSeconds"`
	SessionCount          int                     `json:"sessionCount"`
	AverageSessionSeconds int64                   `json:"averageSessionSeconds"`
	Sessions              []TimeStatisticsSession `json:"sessions"`
}

type TimeActivityStatisticsRange struct {
	Now       time.Time
	WeekStart time.Time
}

type GitHubActivityDay struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
	Level int    `json:"level"`
}

type GitHubTrackerState struct {
	Configured         bool                `json:"configured"`
	Username           string              `json:"username"`
	TodayContributions int                 `json:"todayContributions"`
	WeekContributions  int                 `json:"weekContributions"`
	YearContributions  int                 `json:"yearContributions"`
	Days               []GitHubActivityDay `json:"days"`
	FetchedAt          string              `json:"fetchedAt"`
	Stale              bool                `json:"stale"`
	Source             string              `json:"source"`
}

type GitHubTrackerRecord struct {
	Username  string
	Activity  GitHubTrackerState
	FetchedAt string
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
	Meal         string           `json:"meal"`
	Calories     float64          `json:"calories"`
	Carbohydrate float64          `json:"carbohydrate"`
	Protein      float64          `json:"protein"`
	Fat          float64          `json:"fat"`
	EntryCount   int              `json:"entryCount"`
	Entries      []NutritionEntry `json:"entries"`
}

type NutritionEntry struct {
	ID            string  `json:"id"`
	FoodID        string  `json:"foodId"`
	ServingID     string  `json:"servingId"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Meal          string  `json:"meal"`
	NumberOfUnits float64 `json:"numberOfUnits"`
	Calories      float64 `json:"calories"`
	Carbohydrate  float64 `json:"carbohydrate"`
	Protein       float64 `json:"protein"`
	Fat           float64 `json:"fat"`
}

type FoodServing struct {
	ID            string  `json:"id"`
	Description   string  `json:"description"`
	MetricAmount  float64 `json:"metricAmount,omitempty"`
	MetricUnit    string  `json:"metricUnit,omitempty"`
	NumberOfUnits float64 `json:"numberOfUnits"`
	Measurement   string  `json:"measurement,omitempty"`
	Calories      float64 `json:"calories"`
	Carbohydrate  float64 `json:"carbohydrate"`
	Protein       float64 `json:"protein"`
	Fat           float64 `json:"fat"`
}

type Food struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	BrandName   string        `json:"brandName,omitempty"`
	Type        string        `json:"type"`
	Description string        `json:"description,omitempty"`
	ServingID   string        `json:"servingId,omitempty"`
	Units       float64       `json:"units,omitempty"`
	Servings    []FoodServing `json:"servings,omitempty"`
}

type FoodSearchPage struct {
	Foods   []Food
	Page    int
	HasMore bool
}

type FoodSearchResult struct {
	Query          string `json:"query"`
	CorrectedQuery string `json:"correctedQuery,omitempty"`
	Foods          []Food `json:"foods"`
	Page           int    `json:"page"`
	HasMore        bool   `json:"hasMore"`
}

type FoodProvider string

const (
	FoodProviderLocal         FoodProvider = "local"
	FoodProviderOpenFoodFacts FoodProvider = "open_food_facts"
	FoodProviderFatSecret     FoodProvider = "fatsecret"
	FoodProviderCommunity     FoodProvider = "community"
)

type FoodCatalogItem struct {
	ID                  string        `json:"id"`
	Provider            FoodProvider  `json:"provider"`
	ExternalID          string        `json:"externalId"`
	Barcode             string        `json:"barcode,omitempty"`
	Name                string        `json:"name"`
	BrandName           string        `json:"brandName,omitempty"`
	Description         string        `json:"description,omitempty"`
	CaloriesPer100G     float64       `json:"caloriesPer100g"`
	CarbohydratePer100G float64       `json:"carbohydratePer100g"`
	ProteinPer100G      float64       `json:"proteinPer100g"`
	FatPer100G          float64       `json:"fatPer100g"`
	NutritionUnit       string        `json:"nutritionUnit,omitempty"`
	PortionAmount       float64       `json:"portionAmount,omitempty"`
	Servings            []FoodServing `json:"servings"`
	DataQuality         float64       `json:"dataQuality"`
	ServingID           string        `json:"servingId,omitempty"`
	Units               float64       `json:"units,omitempty"`
}

type FoodCatalogSearchResult struct {
	Query               string            `json:"query"`
	CorrectedQuery      string            `json:"correctedQuery,omitempty"`
	Foods               []FoodCatalogItem `json:"foods"`
	Page                int               `json:"page"`
	HasMore             bool              `json:"hasMore"`
	ExternalUnavailable bool              `json:"externalUnavailable,omitempty"`
}

type FoodCatalogCandidateInput struct {
	Name                string
	BrandName           string
	CaloriesPer100G     float64
	CarbohydratePer100G float64
	ProteinPer100G      float64
	FatPer100G          float64
	NutritionUnit       string
}

type FoodCatalogReviewItem struct {
	FoodCatalogItem
	ReviewID    int64  `json:"reviewId"`
	OwnerLogin  string `json:"ownerLogin"`
	SubmittedAt string `json:"submittedAt"`
	Promoted    bool   `json:"promoted"`
	PromotedAt  string `json:"promotedAt,omitempty"`
}

type FoodCatalogReviewInput struct {
	Barcode             string  `json:"barcode"`
	Name                string  `json:"name"`
	BrandName           string  `json:"brandName"`
	Description         string  `json:"description"`
	CaloriesPer100G     float64 `json:"caloriesPer100g"`
	CarbohydratePer100G float64 `json:"carbohydratePer100g"`
	ProteinPer100G      float64 `json:"proteinPer100g"`
	FatPer100G          float64 `json:"fatPer100g"`
}

type FoodCatalogReviewPage struct {
	Foods   []FoodCatalogReviewItem `json:"foods"`
	Page    int                     `json:"page"`
	HasMore bool                    `json:"hasMore"`
}

type LocalNutritionEntryInput struct {
	FoodID        string  `json:"foodId"`
	ServingID     string  `json:"servingId"`
	Name          string  `json:"name"`
	Meal          string  `json:"meal"`
	Date          string  `json:"date"`
	NumberOfUnits float64 `json:"numberOfUnits"`
}

type FoodEntryInput struct {
	FoodID        string  `json:"foodId"`
	Name          string  `json:"name"`
	ServingID     string  `json:"servingId"`
	NumberOfUnits float64 `json:"numberOfUnits"`
	Meal          string  `json:"meal"`
	Date          string  `json:"date"`
}

type FoodEntryUpdate struct {
	Name          string  `json:"name"`
	ServingID     string  `json:"servingId"`
	NumberOfUnits float64 `json:"numberOfUnits"`
	Meal          string  `json:"meal"`
}

type State struct {
	Profile              Profile              `json:"profile"`
	ActiveTasks          []Task               `json:"activeTasks"`
	CurrentDate          string               `json:"currentDate"`
	BottomNavigation     []string             `json:"bottomNavigation"`
	WorkspacePreferences WorkspacePreferences `json:"workspacePreferences"`
}

type FatSecretStatus struct {
	Configured  bool   `json:"configured"`
	Connected   bool   `json:"connected"`
	ConnectedAt string `json:"connectedAt"`
}
