export interface AuthUser {
  id: number;
  login: string;
  createdAt: string;
  isAdmin: boolean;
}

export interface AuthResponse {
  user: AuthUser;
  expiresAt?: string;
}

export type TaskStatus = "todo" | "done";
export type TaskRecurrenceType = "" | "daily" | "weekly" | "monthly";

export interface Profile {
  name: string;
  surname: string;
  occupation: string;
  sex: string;
  dob: string;
  expiry: string;
  photo: string;
  signature: string;
  workDisplayName: string;
  workAvatar: string;
  workShowAvatar: boolean;
}

export interface WorkProfileInput { displayName: string; avatar: string; showAvatar: boolean }

export interface Task {
  id: number;
  title: string;
  description: string;
  category: string;
  status: TaskStatus;
  dueDate: string;
  dueTime: string;
  reminderAt: string;
  reminderSentAt: string;
  priority: number;
  createdAt: string;
  completedAt: string;
  isMilestone: boolean;
  recurrenceType: TaskRecurrenceType;
  recurrenceInterval: number;
  recurrenceEndDate: string;
  recurrenceWeekdays: number[];
  sortOrder: number;
}

export interface TaskInput {
  title: string;
  description: string;
  category: string;
  status: TaskStatus;
  dueDate: string;
  dueTime: string;
  reminderAt: string;
  priority: number;
  isMilestone: boolean;
  recurrenceType: TaskRecurrenceType;
  recurrenceInterval: number;
  recurrenceEndDate: string;
  recurrenceWeekdays: number[];
  projectIds?: number[];
}

export interface Goal {
  id: number;
  title: string;
  description: string;
  summary: string;
  currentValue: number;
  targetValue: number;
  unit: string;
  deadline: string;
  relatedTaskIds: string[];
  completed: boolean;
  completedAt: string;
  pinned: boolean;
  sortOrder: number;
  completionPct: number;
  createdAt: string;
  updatedAt: string;
}

export interface GoalInput {
  title: string;
  description: string;
  summary: string;
  currentValue: number;
  targetValue: number;
  unit: string;
  deadline: string;
  relatedTaskIds: string[];
  completed: boolean;
  pinned: boolean;
}

export type WorkProjectStatus = "on_track" | "at_risk" | "off_track";
export interface WorkMember { userId: number; name: string; avatar: string; role: "owner" | "member" }
export interface WorkProject {
  id: number; title: string; description: string; summary: string; status: WorkProjectStatus;
  deadline: string; completed: boolean; completedAt: string; pinned: boolean; ownerId: number;
  role: "owner" | "member"; taskCount: number; completedCount: number; completionPct: number;
  contentRevision: number; version: number; createdAt: string; updatedAt: string; members: WorkMember[];
}
export interface WorkSection { id: number; projectId: number; title: string; sortOrder: number; completedSectionId: number | null; resetCompletedOnMove: boolean; moveCompletedToEnd: boolean }
export interface WorkSectionInput { title: string; relativeTo?: number | null; placement?: "above" | "below" | "" }
export interface WorkTask {
  id: number; projectId: number; sectionId: number | null; parentId: number | null; title: string;
  description: string; createdById: number | null; assigneeId: number | null; assignee: string; dueDate: string; completed: boolean;
  sortOrder: number; version: number; createdAt: string; updatedAt: string;
}
export interface WorkComment { id: number; taskId: number; authorId: number; author: string; avatar: string; body: string; createdAt: string }
export interface WorkAttachment { id: number; taskId: number | null; projectId: number; name: string; mimeType: string; size: number; createdAt: string }
export interface WorkLink { id: number; projectId: number; title: string; url: string; createdAt: string }
export interface WorkNote { id: number; projectId: number; title: string; body: string; version: number; createdAt: string; updatedAt: string }
export interface WorkNoteSummary { id: number; projectId: number; title: string; preview: string; version: number; createdAt: string; updatedAt: string }
export interface WorkEvent { id: number; actorId: number | null; actor: string; actorAvatar: string; kind: string; message: string; createdAt: string }
export interface WorkProjectDetail { project: WorkProject; sections: WorkSection[]; tasks: WorkTask[]; comments: WorkComment[]; attachments: WorkAttachment[]; links: WorkLink[]; notes: WorkNote[]; events: WorkEvent[] }
export interface WorkProjectResources { revision: number; comments: WorkComment[]; attachments: WorkAttachment[]; links: WorkLink[]; notes: WorkNote[]; events: WorkEvent[] }
export interface WorkProjectResourceSummary { revision: number; attachments: WorkAttachment[]; links: WorkLink[]; notes: WorkNoteSummary[]; events: WorkEvent[] }
export type WorkProjectInput = Pick<WorkProject, "title" | "description" | "summary" | "status" | "deadline" | "completed" | "pinned" | "version">;
export type WorkTaskInput = Pick<WorkTask, "sectionId" | "parentId" | "title" | "description" | "assigneeId" | "dueDate" | "completed" | "sortOrder" | "version">;
export type WorkNoteInput = Pick<WorkNote, "title" | "body" | "version">;

export interface StateResponse {
  profile: Profile;
  activeTasks: Task[];
  currentDate: string;
  bottomNavigation: BottomNavigationItem[];
  workspacePreferences: WorkspacePreferences;
}

export type WorkspaceFeature = "tracker" | "calories" | "tasks" | "projects" | "widgets";

export interface WorkspacePreferences {
  features: WorkspaceFeature[];
  onboardingCompleted: boolean;
  timerIntroSeen: boolean;
}

export type BottomNavigationItem = "card" | "tasks" | "projects" | "tracker" | "calories" | "profile";

export interface PortfolioResponse {
  pinned: Goal[];
  active: Goal[];
  completed: Goal[];
}

export interface TrackerWeightEntry {
  date: string;
  weightKg: number;
  updatedAt: string;
}

export interface TrackerWaterEntry {
  date: string;
  glasses: number;
  goalGlasses: number;
  updatedAt: string;
}

export interface CustomTracker {
  id: number;
  name: string;
  targetValue: number;
  stepValue: number;
  currentValue: number;
  icon: string;
  createdAt: string;
  updatedAt: string;
}

export interface CustomTrackerInput {
  name: string;
  targetValue: number;
  stepValue: number;
  icon: string;
}

export interface CustomTrackerEntry {
  trackerId: number;
  date: string;
  value: number;
  targetValue: number;
  updatedAt: string;
}

export interface TaskCategory {
  id: number;
  name: string;
  builtin: boolean;
}

export interface NotificationConfig {
  configured: boolean;
  publicKey: string;
}

export interface TrackerReminder {
  trackerKey: string;
  time: string;
  enabled: boolean;
}

export interface TrackerReminderInput {
  trackerKey: string;
  time: string;
  enabled: boolean;
}

export interface PushSubscriptionInput {
  endpoint: string;
  p256dh: string;
  auth: string;
}

export interface TrackerState {
  waterGoal: number;
  calorieGoal: number;
  proteinGoal: number;
  fatGoal: number;
  carbohydrateGoal: number;
  currentWeightKg: number | null;
  weightHistory: TrackerWeightEntry[];
  waterHistory: TrackerWaterEntry[];
  customTrackers: CustomTracker[];
  customHistory: CustomTrackerEntry[];
}

export interface NutritionGoals {
  calorieGoal: number;
  proteinGoal: number;
  fatGoal: number;
  carbohydrateGoal: number;
}

export type TimeTrackerPeriod = "today" | "week";

export interface TimeActivity {
  id: number;
  name: string;
  todaySeconds: number;
  periodSeconds: number;
  lastStartedDate?: string;
  active: boolean;
  sortOrder: number;
  createdAt: string;
  reminderIntervalMinutes: number;
  reminderMessage: string;
}

export interface TimeActivityInput {
  name: string;
  reminderIntervalMinutes: number;
  reminderMessage: string;
}

export interface ActiveTimeSession {
  id: number;
  activityId: number;
  activityName: string;
  startedAt: string;
  elapsedSeconds: number;
}

export interface TimeTrackerState {
  date: string;
  period: TimeTrackerPeriod;
  activities: TimeActivity[];
  activeSession: ActiveTimeSession | null;
  pausedSession: ActiveTimeSession | null;
  todayTotalSeconds: number;
  periodTotalSeconds: number;
}

export type TimeStatisticsPeriod = "today" | "week" | "month";

export interface TimeStatisticsActivity {
  id: number;
  name: string;
  totalSeconds: number;
  sessionCount: number;
  sharePercent: number;
}

export interface TimeStatisticsDay {
  date: string;
  totalSeconds: number;
}

export interface TimeStatisticsSession {
  id: number;
  activityId: number;
  activityName: string;
  date: string;
  startedAt: string;
  endedAt?: string;
  durationSeconds: number;
  active: boolean;
}

export interface TimeStatistics {
  date: string;
  period: TimeStatisticsPeriod;
  periodStart: string;
  periodEnd: string;
  totalSeconds: number;
  activityCount: number;
  sessionCount: number;
  longestSessionSeconds: number;
  activeDays: number;
  activities: TimeStatisticsActivity[];
  days: TimeStatisticsDay[];
  sessions: TimeStatisticsSession[];
}

export interface TimeActivityStatistics {
  activityId: number;
  name: string;
  totalSeconds: number;
  weekSeconds: number;
  sessionCount: number;
  averageSessionSeconds: number;
  sessions: TimeStatisticsSession[];
}

export interface GitHubActivityDay {
  date: string;
  count: number;
  level: number;
}

export interface GitHubTrackerState {
  configured: boolean;
  username: string;
  todayContributions: number;
  weekContributions: number;
  yearContributions: number;
  days: GitHubActivityDay[];
  fetchedAt: string;
  stale: boolean;
  source: string;
}

export interface FatSecretStatus {
  configured: boolean;
  connected: boolean;
  connectedAt: string;
}

export interface FatSecretMealNutrition {
  meal: string;
  calories: number;
  carbohydrate: number;
  protein: number;
  fat: number;
  entryCount: number;
  entries: FatSecretNutritionEntry[];
}

export interface FatSecretNutritionEntry {
  id: string;
  foodId: string;
  servingId: string;
  name: string;
  description: string;
  meal: string;
  numberOfUnits: number;
  calories: number;
  carbohydrate: number;
  protein: number;
  fat: number;
}

export interface FatSecretFoodServing {
  id: string;
  description: string;
  metricAmount?: number;
  metricUnit?: string;
  numberOfUnits: number;
  measurement?: string;
  calories: number;
  carbohydrate: number;
  protein: number;
  fat: number;
}

export interface FatSecretFood {
  id: string;
  provider?: string;
  name: string;
  brandName?: string;
  type: string;
  description?: string;
  servingId?: string;
  units?: number;
  servings?: FatSecretFoodServing[];
}

export interface LocalFood extends FatSecretFood {
  provider: string;
  externalId: string;
  barcode?: string;
  caloriesPer100g: number;
  carbohydratePer100g: number;
  proteinPer100g: number;
  fatPer100g: number;
  nutritionUnit?: "g" | "ml";
  portionAmount?: number;
  dataQuality: number;
}

export interface FoodCatalogSearchResult {
  query: string;
  correctedQuery?: string;
  foods: LocalFood[];
  page: number;
  hasMore: boolean;
  externalUnavailable?: boolean;
}

export interface FoodCatalogCandidateInput {
  name: string;
  brandName: string;
  caloriesPer100g: number;
  carbohydratePer100g: number;
  proteinPer100g: number;
  fatPer100g: number;
  nutritionUnit: "g" | "ml";
}

export interface UserFoodReview extends LocalFood {
  reviewId: number;
  ownerLogin: string;
  submittedAt: string;
  promoted: boolean;
  promotedAt?: string;
}

export interface FoodCatalogReviewInput {
  barcode: string;
  name: string;
  brandName: string;
  description: string;
  caloriesPer100g: number;
  carbohydratePer100g: number;
  proteinPer100g: number;
  fatPer100g: number;
}

export interface UserFoodReviewPage {
  foods: UserFoodReview[];
  page: number;
  hasMore: boolean;
}

export interface AdminLogsResponse {
  lines: string[];
}

export interface FatSecretFoodEntryInput {
  foodId: string;
  name: string;
  servingId: string;
  numberOfUnits: number;
  meal: FatSecretMeal;
  date: string;
}

export type FatSecretMeal = "breakfast" | "lunch" | "dinner" | "other";

export interface FatSecretNutrition {
  date: string;
  calories: number;
  carbohydrate: number;
  protein: number;
  fat: number;
  entryCount: number;
  meals: FatSecretMealNutrition[];
  fetchedAt: string;
}
