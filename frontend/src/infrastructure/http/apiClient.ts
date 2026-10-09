import type {
  AuthResponse,
  AdminLogsResponse,
  BottomNavigationItem,
  WorkspacePreferences,
  CustomTracker,
  CustomTrackerInput,
  FatSecretFood,
  FatSecretFoodEntryInput,
  FatSecretMeal,
  FatSecretNutrition,
  FatSecretStatus,
  FoodCatalogCandidateInput,
  FoodCatalogSearchResult,
  FoodCatalogReviewInput,
  GitHubTrackerState,
  Goal,
  GoalInput,
  LocalFood,
  NotificationConfig,
  NutritionGoals,
  PortfolioResponse,
  Profile,
  PushSubscriptionInput,
  StateResponse,
  Task,
  TaskCategory,
  TaskInput,
  TimeActivity,
  TimeActivityStatistics,
  TimeStatistics,
  TimeStatisticsPeriod,
  TimeTrackerPeriod,
  TimeTrackerState,
  TimeActivityInput,
  TrackerReminder,
  TrackerReminderInput,
  TrackerState,
  TrackerWaterEntry,
  TrackerWeightEntry,
  UserFoodReviewPage,
  UserFoodReview,
  WorkProject,
  WorkProjectDetail,
  WorkProjectResources,
  WorkProjectResourceSummary,
  WorkProjectInput,
  WorkProfileInput,
  WorkSection,
  WorkSectionInput,
  WorkTask,
  WorkTaskInput,
  WorkComment,
  WorkAttachment,
  WorkLink,
  WorkNote,
  WorkNoteInput,
} from "../../domain/models";

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "ApiError";
  }
}

const MAX_WORK_ATTACHMENT_BYTES = 50 * 1024 * 1024;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  const headers = new Headers(init?.headers);
  if (init?.body !== undefined && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    headers.set("X-AVATAR-ID-Request", "1");
  }
  let response: Response;
  try {
    response = await fetch(path, {
      credentials: "same-origin",
      ...init,
      method,
      headers,
    });
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === "AbortError") {
      throw new ApiError(0, "Запрос был отменён");
    }
    if (typeof navigator !== "undefined" && navigator.onLine === false) {
      throw new ApiError(0, "Отсутствует подключение к интернету");
    }
    throw new ApiError(0, "Не удалось подключиться к серверу. Проверьте интернет-соединение");
  }
  if (!response.ok) {
    const message = (await response.text()).trim();
    if (response.status === 401 && !path.startsWith("/api/auth/")) {
      window.dispatchEvent(new CustomEvent("avatar-id:unauthorized"));
    }
    throw new ApiError(response.status, message || `${response.status} ${response.statusText}`);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

// Infrastructure adapter for the backend HTTP API. All endpoints remain
// relative so the website and installed PWA keep the same origin and cookies.
export const api = {
  authSession: () => request<AuthResponse>("/api/auth/session", { cache: "no-store" }),
  registrationConfig: () => request<{ enabled: boolean }>("/api/auth/registration", { cache: "no-store" }),
  requestRegistration: (email: string) => request<{ message: string }>("/api/auth/registration/request", { method: "POST", body: JSON.stringify({ email }) }),
  completeRegistration: (token: string, login: string, password: string) => request<void>("/api/auth/registration/complete", { method: "POST", body: JSON.stringify({ token, login, password }) }),
  login: (login: string, password: string) =>
    request<AuthResponse>("/api/auth/login", { method: "POST", body: JSON.stringify({ login, password }) }),
  logout: () => request<void>("/api/auth/logout", { method: "POST" }),
  state: (includeActiveTasks = true) => request<StateResponse>(`/api/state?includeActiveTasks=${includeActiveTasks}`),
  fatSecretStatus: () => request<FatSecretStatus>("/api/integrations/fatsecret/status"),
  fatSecretNutrition: (date: string) =>
    request<FatSecretNutrition>(`/api/integrations/fatsecret/nutrition?date=${encodeURIComponent(date)}&_=${Date.now()}`, { cache: "no-store" }),
  fatSecretFood: (foodId: string) =>
    request<FatSecretFood>(`/api/integrations/fatsecret/foods/${encodeURIComponent(foodId)}`, { cache: "no-store" }),
  fatSecretBarcodeFood: (barcode: string) =>
    request<FatSecretFood>(`/api/integrations/fatsecret/foods/barcode/${encodeURIComponent(barcode)}`, { cache: "no-store" }),
  recentFatSecretFoods: (meal?: FatSecretMeal) =>
    request<FatSecretFood[]>(`/api/integrations/fatsecret/foods/recent${meal ? `?meal=${encodeURIComponent(meal)}` : ""}`, { cache: "no-store" }),
  createFatSecretEntry: (input: FatSecretFoodEntryInput) =>
    request<FatSecretNutrition>("/api/integrations/fatsecret/entries", { method: "POST", body: JSON.stringify(input) }),
  updateFatSecretEntry: (entryId: string, date: string, input: Omit<FatSecretFoodEntryInput, "foodId" | "date">) =>
    request<FatSecretNutrition>(`/api/integrations/fatsecret/entries/${encodeURIComponent(entryId)}?date=${encodeURIComponent(date)}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteFatSecretEntry: (entryId: string, date: string) =>
    request<FatSecretNutrition>(`/api/integrations/fatsecret/entries/${encodeURIComponent(entryId)}?date=${encodeURIComponent(date)}`, { method: "DELETE" }),
  localNutrition: (date: string) => request<FatSecretNutrition>(`/api/nutrition/local?date=${encodeURIComponent(date)}`, { cache: "no-store" }),
  searchCatalogFoods: (query: string, options: { page?: number; exact?: boolean; signal?: AbortSignal } = {}) => {
    const page = options.page ?? 0;
    return request<FoodCatalogSearchResult>(`/api/foods/search?q=${encodeURIComponent(query)}&page=${page}${options.exact ? "&exact=1" : ""}`, {
      cache: "no-store",
      signal: options.signal,
    });
  },
  recentLocalFoods: (meal?: FatSecretMeal) =>
    request<LocalFood[]>(`/api/foods/recent${meal ? `?meal=${encodeURIComponent(meal)}` : ""}`, { cache: "no-store" }),
  localFoodBarcode: (barcode: string) => request<LocalFood>(`/api/foods/barcode?value=${encodeURIComponent(barcode)}`, { cache: "no-store" }),
  localFoodCandidates: (input: FoodCatalogCandidateInput) => {
    const query = new URLSearchParams({
      name: input.name,
      brand: input.brandName,
      calories: String(input.caloriesPer100g),
      protein: String(input.proteinPer100g),
      fat: String(input.fatPer100g),
      carbohydrate: String(input.carbohydratePer100g),
      unit: input.nutritionUnit,
    });
    return request<LocalFood[]>(`/api/foods/candidates?${query}`, { cache: "no-store" });
  },
  linkLocalFoodBarcode: (barcode: string, foodId: string) =>
    request<LocalFood>("/api/foods/barcode-links", { method: "POST", body: JSON.stringify({ barcode, foodId }) }),
  localFood: (foodId: string) => request<LocalFood>(`/api/foods/${encodeURIComponent(foodId)}`, { cache: "no-store" }),
  createLocalFood: (input: Partial<LocalFood>) => request<LocalFood>("/api/foods", { method: "POST", body: JSON.stringify(input) }),
  deleteLocalFood: (foodId: string) => request<void>(`/api/foods/${encodeURIComponent(foodId)}`, { method: "DELETE" }),
  createLocalNutritionEntry: (input: FatSecretFoodEntryInput) => request<FatSecretNutrition>("/api/nutrition/local/entries", { method: "POST", body: JSON.stringify(input) }),
  updateLocalNutritionEntry: (entryId: string, input: FatSecretFoodEntryInput) => request<FatSecretNutrition>(`/api/nutrition/local/entries/${encodeURIComponent(entryId)}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteLocalNutritionEntry: (entryId: string, date: string) => request<FatSecretNutrition>(`/api/nutrition/local/entries/${encodeURIComponent(entryId)}?date=${encodeURIComponent(date)}`, { method: "DELETE" }),
  adminUserFoods: (page = 0, pendingOnly = true) =>
    request<UserFoodReviewPage>(`/api/admin/foods?page=${page}&status=${pendingOnly ? "pending" : "all"}`, { cache: "no-store" }),
  updateUserFood: (reviewId: number, input: FoodCatalogReviewInput) =>
    request<UserFoodReview>(`/api/admin/foods/${reviewId}`, { method: "PUT", body: JSON.stringify(input) }),
  promoteUserFood: (reviewId: number) =>
    request<LocalFood>(`/api/admin/foods/${reviewId}/promote`, { method: "POST" }),
  rejectUserFood: (reviewId: number) =>
    request<void>(`/api/admin/foods/${reviewId}`, { method: "DELETE" }),
  adminLogs: (limit = 200) =>
    request<AdminLogsResponse>(`/api/admin/logs?limit=${limit}`, { cache: "no-store" }),
  connectFatSecret: (returnTo: string) =>
    request<{ authorizeUrl: string }>(`/api/integrations/fatsecret/connect?return_to=${encodeURIComponent(returnTo)}`, { method: "POST" }),
  disconnectFatSecret: () => request<void>("/api/integrations/fatsecret", { method: "DELETE" }),
  trackers: () => request<TrackerState>("/api/trackers"),
  githubTracker: (refresh = false) =>
    request<GitHubTrackerState>(`/api/trackers/github${refresh ? "?refresh=1" : ""}`, { cache: "no-store" }),
  saveGitHubTracker: (username: string) =>
    request<GitHubTrackerState>("/api/trackers/github", { method: "PUT", body: JSON.stringify({ username }) }),
  deleteGitHubTracker: () => request<void>("/api/trackers/github", { method: "DELETE" }),
  saveTrackerWeight: (date: string, weightKg: number) =>
    request<TrackerWeightEntry>(`/api/trackers/weight/${encodeURIComponent(date)}`, {
      method: "PUT",
      body: JSON.stringify({ weightKg }),
    }),
  saveNutritionGoals: (goals: NutritionGoals) =>
    request<NutritionGoals>("/api/trackers/calorie-goal", {
      method: "PUT",
      body: JSON.stringify(goals),
    }),
  saveTrackerWater: (date: string, glasses: number, goalGlasses: number) =>
    request<TrackerWaterEntry>(`/api/trackers/water/${encodeURIComponent(date)}`, {
      method: "PUT",
      body: JSON.stringify({ glasses, goalGlasses }),
    }),
  createCustomTracker: (input: CustomTrackerInput) =>
    request<CustomTracker>("/api/trackers/custom", { method: "POST", body: JSON.stringify(input) }),
  updateCustomTracker: (id: number, input: CustomTrackerInput) =>
    request<CustomTracker>(`/api/trackers/custom/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  stepCustomTracker: (id: number, date: string, direction: -1 | 1) =>
    request<CustomTracker>(`/api/trackers/custom/${id}/step`, { method: "POST", body: JSON.stringify({ date, direction }) }),
  deleteCustomTracker: (id: number) => request<void>(`/api/trackers/custom/${id}`, { method: "DELETE" }),
  trackerReminders: () => request<TrackerReminder[]>("/api/trackers/reminders", { cache: "no-store" }),
  saveTrackerReminder: (input: TrackerReminderInput) =>
    request<TrackerReminder>("/api/trackers/reminders", { method: "PUT", body: JSON.stringify(input) }),
  deleteTrackerReminder: (trackerKey: string) =>
    request<void>("/api/trackers/reminders", { method: "DELETE", body: JSON.stringify({ trackerKey }) }),
  timeTracker: (period: TimeTrackerPeriod) =>
    request<TimeTrackerState>(`/api/time-tracker?period=${period}`, { cache: "no-store" }),
  createTimeActivity: (input: TimeActivityInput) =>
    request<TimeActivity>("/api/time-tracker/activities", { method: "POST", body: JSON.stringify(input) }),
  updateTimeActivity: (id: number, input: TimeActivityInput) =>
    request<TimeActivity>(`/api/time-tracker/activities/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteTimeActivity: (id: number) =>
    request<void>(`/api/time-tracker/activities/${id}`, { method: "DELETE" }),
  startTimeActivity: (id: number, period: TimeTrackerPeriod) =>
    request<TimeTrackerState>(`/api/time-tracker/activities/${id}/start?period=${period}`, { method: "POST" }),
  pauseTimeActivity: (period: TimeTrackerPeriod) =>
    request<TimeTrackerState>(`/api/time-tracker/pause?period=${period}`, { method: "POST" }),
  finishTimeActivity: (period: TimeTrackerPeriod) =>
    request<TimeTrackerState>(`/api/time-tracker/finish?period=${period}`, { method: "POST" }),
  timeStatistics: (period: TimeStatisticsPeriod) =>
    request<TimeStatistics>(`/api/time-tracker/statistics?period=${period}`, { cache: "no-store" }),
  timeActivityStatistics: (id: number) =>
    request<TimeActivityStatistics>(`/api/time-tracker/statistics/activities/${id}`, { cache: "no-store" }),
  taskCategories: () => request<TaskCategory[]>("/api/task-categories"),
  createTaskCategory: (name: string) =>
    request<TaskCategory>("/api/task-categories", { method: "POST", body: JSON.stringify({ name }) }),
  deleteTaskCategory: (id: number) => request<void>(`/api/task-categories/${id}`, { method: "DELETE" }),
  notificationConfig: () => request<NotificationConfig>("/api/notifications/config", { cache: "no-store" }),
  savePushSubscription: (input: PushSubscriptionInput) =>
    request<void>("/api/notifications/subscriptions", { method: "POST", body: JSON.stringify(input) }),
  deletePushSubscription: (endpoint: string) =>
    request<void>("/api/notifications/subscriptions", { method: "DELETE", body: JSON.stringify({ endpoint }) }),
  tasks: () => request<Task[]>("/api/tasks"),
  createTask: (input: TaskInput) =>
    request<Task>("/api/tasks", { method: "POST", body: JSON.stringify(input) }),
  updateTask: (id: number, input: TaskInput) =>
    request<Task>(`/api/tasks/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  swapTaskOrder: (id: number, otherId: number) =>
    request<Task[]>("/api/tasks/order", { method: "PUT", body: JSON.stringify({ id, otherId }) }),
  deleteTask: (id: number) => request<void>(`/api/tasks/${id}`, { method: "DELETE" }),
  completeTask: (id: number) =>
    request<Task>(`/api/tasks/${id}/complete`, { method: "POST" }),
  uncompleteTask: (id: number) =>
    request<Task>(`/api/tasks/${id}/complete`, { method: "DELETE" }),
  goals: () => request<Goal[]>("/api/goals"),
  createGoal: (input: GoalInput) =>
    request<Goal>("/api/goals", { method: "POST", body: JSON.stringify(input) }),
  updateGoal: (id: number, input: GoalInput) =>
    request<Goal>(`/api/goals/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteGoal: (id: number) => request<void>(`/api/goals/${id}`, { method: "DELETE" }),
  reorderGoals: (ids: number[]) => request<Goal[]>("/api/goals/order", { method: "PUT", body: JSON.stringify({ ids }) }),
  portfolio: () => request<PortfolioResponse>("/api/portfolio"),
  workProjects: () => request<WorkProject[]>("/api/work/projects", { cache: "no-store" }),
  workProject: (id: number) => request<WorkProjectDetail>(`/api/work/projects/${id}`, { cache: "no-store" }),
  workProjectCore: (id: number) => request<WorkProjectDetail>(`/api/work/projects/${id}/core`, { cache: "no-store" }),
  workProjectRevision: (id: number) => request<{ revision: number }>(`/api/work/projects/${id}/revision`, { cache: "no-store" }),
  workProjectResources: (id: number) => request<WorkProjectResources>(`/api/work/projects/${id}/resources`, { cache: "no-store" }),
  workProjectResourceSummary: (id: number) => request<WorkProjectResourceSummary>(`/api/work/projects/${id}/resource-summary`, { cache: "no-store" }),
  createWorkProject: (input: WorkProjectInput) => request<WorkProjectDetail>("/api/work/projects", { method: "POST", body: JSON.stringify(input) }),
  updateWorkProject: (id: number, input: WorkProjectInput) => request<WorkProjectDetail>(`/api/work/projects/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteWorkProject: (id: number) => request<void>(`/api/work/projects/${id}`, { method: "DELETE" }),
  createWorkSection: (projectId: number, input: WorkSectionInput) => request<WorkSection>(`/api/work/projects/${projectId}/sections`, { method: "POST", body: JSON.stringify(input) }),
  updateWorkSection: (id: number, title: string) => request<WorkSection>(`/api/work/sections/${id}`, { method: "PUT", body: JSON.stringify({ title }) }),
  updateWorkSectionCompletion: (id: number, sectionId: number | null, resetCompletedOnMove: boolean, moveCompletedToEnd: boolean) => request<WorkSection>(`/api/work/sections/${id}/completion-target`, { method: "PUT", body: JSON.stringify({ sectionId, resetCompletedOnMove, moveCompletedToEnd }) }),
  deleteWorkSection: (id: number) => request<void>(`/api/work/sections/${id}`, { method: "DELETE" }),
  createWorkTask: (projectId: number, input: WorkTaskInput) => request<WorkTask>(`/api/work/projects/${projectId}/tasks`, { method: "POST", body: JSON.stringify(input) }),
  updateWorkTask: (id: number, input: WorkTaskInput) => request<WorkTask>(`/api/work/tasks/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  claimWorkTask: (id: number) => request<WorkTask>(`/api/work/tasks/${id}/claim`, { method: "POST" }),
  deleteWorkTask: (id: number) => request<void>(`/api/work/tasks/${id}`, { method: "DELETE" }),
  createWorkComment: (taskId: number, body: string) => request<WorkComment>(`/api/work/tasks/${taskId}/comments`, { method: "POST", body: JSON.stringify({ body }) }),
  workTaskComments: (taskId: number) => request<WorkComment[]>(`/api/work/tasks/${taskId}/comments`, { cache: "no-store" }),
  createWorkInvite: (projectId: number) => request<{ token: string; expiresAt: string }>(`/api/work/projects/${projectId}/invites`, { method: "POST" }),
  revokeWorkInvites: (projectId: number) => request<void>(`/api/work/projects/${projectId}/invites`, { method: "DELETE" }),
  removeWorkMember: (projectId: number, userId: number) => request<void>(`/api/work/projects/${projectId}/members/${userId}`, { method: "DELETE" }),
  leaveWorkProject: (projectId: number) => request<void>(`/api/work/projects/${projectId}/membership`, { method: "DELETE" }),
  acceptWorkInvite: (token: string) => request<WorkProjectDetail>("/api/work/invites/accept", { method: "POST", body: JSON.stringify({ token }) }),
  uploadWorkAttachment: async (projectId: number, taskId: number | null, file: File) => {
    if (file.size > MAX_WORK_ATTACHMENT_BYTES) {
      throw new ApiError(400, "Файл больше 50 МБ. Загрузите его на Яндекс Диск или Google Диск и добавьте ссылку.");
    }
    const body = new FormData(); body.append("file", file); if (taskId) body.append("taskId", String(taskId));
    return request<WorkAttachment>(`/api/work/projects/${projectId}/attachments`, { method: "POST", body });
  },
  deleteWorkAttachment: (id: number) => request<void>(`/api/work/attachments/${id}`, { method: "DELETE" }),
  createWorkLink: (projectId: number, title: string, url: string) =>
    request<WorkLink>(`/api/work/projects/${projectId}/links`, { method: "POST", body: JSON.stringify({ title, url }) }),
  deleteWorkLink: (id: number) => request<void>(`/api/work/links/${id}`, { method: "DELETE" }),
  createWorkNote: (projectId: number, input: WorkNoteInput) =>
    request<WorkNote>(`/api/work/projects/${projectId}/notes`, { method: "POST", body: JSON.stringify(input) }),
  workNote: (id: number) => request<WorkNote>(`/api/work/notes/${id}`, { cache: "no-store" }),
  updateWorkNote: (id: number, input: WorkNoteInput) =>
    request<WorkNote>(`/api/work/notes/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteWorkNote: (id: number) => request<void>(`/api/work/notes/${id}`, { method: "DELETE" }),
  updateProfile: (profile: Pick<Profile, "name" | "surname" | "occupation" | "sex" | "dob" | "expiry">) =>
    request<void>("/api/profile", { method: "PUT", body: JSON.stringify(profile) }),
  updateWorkProfile: (profile: WorkProfileInput) =>
    request<void>("/api/profile/work-visibility", { method: "PUT", body: JSON.stringify(profile) }),
  updateBottomNavigation: (items: BottomNavigationItem[]) =>
    request<void>("/api/profile/bottom-navigation", { method: "PUT", body: JSON.stringify({ items }) }),
  updateWorkspacePreferences: (preferences: WorkspacePreferences) =>
    request<void>("/api/profile/workspace", { method: "PUT", body: JSON.stringify(preferences) }),
  setPhoto: (data: string) =>
    request<void>("/api/photo", { method: "PUT", body: JSON.stringify({ data }) }),
  setSignature: (data: string) =>
    request<void>("/api/signature", { method: "PUT", body: JSON.stringify({ data }) }),
  reset: () => request<void>("/api/reset", { method: "POST", body: JSON.stringify({ confirm: "RESET" }) }),
};
