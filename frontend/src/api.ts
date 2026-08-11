// Единственная точка общения React-приложения с backend API.

export interface AuthUser {
  id: number;
  login: string;
  createdAt: string;
}

export interface AuthResponse {
  user: AuthUser;
  expiresAt?: string;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "ApiError";
  }
}

export type TaskStatus = "todo" | "done";

export interface Profile {
  name: string;
  surname: string;
  occupation: string;
  sex: string;
  dob: string;
  expiry: string;
  photo: string;
  signature: string;
}

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
  tickTickSyncStatus: "" | "pending" | "synced" | "error";
  tickTickSyncError?: string;
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

export interface StateResponse {
  profile: Profile;
  activeTasks: Task[];
  currentDate: string;
}

export interface PortfolioResponse {
  pinned: Goal[];
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
  currentWeightKg: number | null;
  weightHistory: TrackerWeightEntry[];
  waterHistory: TrackerWaterEntry[];
  customTrackers: CustomTracker[];
  customHistory: CustomTrackerEntry[];
}

export interface FatSecretStatus {
  configured: boolean;
  connected: boolean;
  connectedAt: string;
}

export interface TickTickStatus {
  configured: boolean;
  connected: boolean;
  connectedAt: string;
  projectName: string;
  pendingTasks: number;
  failedTasks: number;
}

export interface TickTickSyncResult {
  synced: number;
  failed: number;
  imported: number;
  updated: number;
  completed: number;
}

export interface FatSecretMealNutrition {
  meal: string;
  calories: number;
  carbohydrate: number;
  protein: number;
  fat: number;
  entryCount: number;
}

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

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  const headers = new Headers(init?.headers);
  if (init?.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    headers.set("X-Identity-Workspace-Request", "1");
  }
  const response = await fetch(path, {
    credentials: "same-origin",
    ...init,
    method,
    headers,
  });
  if (!response.ok) {
    const message = (await response.text()).trim();
    if (response.status === 401 && !path.startsWith("/api/auth/")) {
      window.dispatchEvent(new CustomEvent("identity-workspace:unauthorized"));
    }
    throw new ApiError(response.status, message || `${response.status} ${response.statusText}`);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export const api = {
  authSession: () => req<AuthResponse>("/api/auth/session", { cache: "no-store" }),
  login: (login: string, password: string) =>
    req<AuthResponse>("/api/auth/login", { method: "POST", body: JSON.stringify({ login, password }) }),
  logout: () => req<void>("/api/auth/logout", { method: "POST" }),
  state: () => req<StateResponse>("/api/state"),
  fatSecretStatus: () => req<FatSecretStatus>("/api/integrations/fatsecret/status"),
  fatSecretNutrition: (date: string) =>
    req<FatSecretNutrition>(`/api/integrations/fatsecret/nutrition?date=${encodeURIComponent(date)}&_=${Date.now()}`, { cache: "no-store" }),
  connectFatSecret: (returnTo: string) =>
    req<{ authorizeUrl: string }>(`/api/integrations/fatsecret/connect?return_to=${encodeURIComponent(returnTo)}`, { method: "POST" }),
  disconnectFatSecret: () => req<void>("/api/integrations/fatsecret", { method: "DELETE" }),
  tickTickStatus: () => req<TickTickStatus>("/api/integrations/ticktick/status", { cache: "no-store" }),
  connectTickTick: (returnTo: string) =>
    req<{ authorizeUrl: string }>(`/api/integrations/ticktick/connect?return_to=${encodeURIComponent(returnTo)}`, { method: "POST" }),
  syncTickTick: () => req<TickTickSyncResult>("/api/integrations/ticktick/sync", { method: "POST" }),
  disconnectTickTick: () => req<void>("/api/integrations/ticktick", { method: "DELETE" }),
  trackers: () => req<TrackerState>("/api/trackers"),
  saveTrackerWeight: (date: string, weightKg: number) =>
    req<TrackerWeightEntry>(`/api/trackers/weight/${encodeURIComponent(date)}`, {
      method: "PUT",
      body: JSON.stringify({ weightKg }),
    }),
  saveCalorieGoal: (calorieGoal: number) =>
    req<{ calorieGoal: number }>("/api/trackers/calorie-goal", {
      method: "PUT",
      body: JSON.stringify({ calorieGoal }),
    }),
  saveTrackerWater: (date: string, glasses: number, goalGlasses: number) =>
    req<TrackerWaterEntry>(`/api/trackers/water/${encodeURIComponent(date)}`, {
      method: "PUT",
      body: JSON.stringify({ glasses, goalGlasses }),
    }),
  createCustomTracker: (input: CustomTrackerInput) =>
    req<CustomTracker>("/api/trackers/custom", { method: "POST", body: JSON.stringify(input) }),
  updateCustomTracker: (id: number, input: CustomTrackerInput) =>
    req<CustomTracker>(`/api/trackers/custom/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  stepCustomTracker: (id: number, date: string, direction: -1 | 1) =>
    req<CustomTracker>(`/api/trackers/custom/${id}/step`, { method: "POST", body: JSON.stringify({ date, direction }) }),
  deleteCustomTracker: (id: number) => req<void>(`/api/trackers/custom/${id}`, { method: "DELETE" }),
  trackerReminders: () => req<TrackerReminder[]>("/api/trackers/reminders", { cache: "no-store" }),
  saveTrackerReminder: (input: TrackerReminderInput) =>
    req<TrackerReminder>("/api/trackers/reminders", { method: "PUT", body: JSON.stringify(input) }),
  deleteTrackerReminder: (trackerKey: string) =>
    req<void>("/api/trackers/reminders", { method: "DELETE", body: JSON.stringify({ trackerKey }) }),
  taskCategories: () => req<TaskCategory[]>("/api/task-categories"),
  createTaskCategory: (name: string) =>
    req<TaskCategory>("/api/task-categories", { method: "POST", body: JSON.stringify({ name }) }),
  deleteTaskCategory: (id: number) => req<void>(`/api/task-categories/${id}`, { method: "DELETE" }),
  notificationConfig: () => req<NotificationConfig>("/api/notifications/config", { cache: "no-store" }),
  savePushSubscription: (input: PushSubscriptionInput) =>
    req<void>("/api/notifications/subscriptions", { method: "POST", body: JSON.stringify(input) }),
  deletePushSubscription: (endpoint: string) =>
    req<void>("/api/notifications/subscriptions", { method: "DELETE", body: JSON.stringify({ endpoint }) }),
  tasks: () => req<Task[]>("/api/tasks"),
  createTask: (input: TaskInput) =>
    req<Task>("/api/tasks", { method: "POST", body: JSON.stringify(input) }),
  updateTask: (id: number, input: TaskInput) =>
    req<Task>(`/api/tasks/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteTask: (id: number) => req<void>(`/api/tasks/${id}`, { method: "DELETE" }),
  completeTask: (id: number) =>
    req<Task>(`/api/tasks/${id}/complete`, { method: "POST" }),
  uncompleteTask: (id: number) =>
    req<Task>(`/api/tasks/${id}/complete`, { method: "DELETE" }),

  goals: () => req<Goal[]>("/api/goals"),
  createGoal: (input: GoalInput) =>
    req<Goal>("/api/goals", { method: "POST", body: JSON.stringify(input) }),
  updateGoal: (id: number, input: GoalInput) =>
    req<Goal>(`/api/goals/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteGoal: (id: number) => req<void>(`/api/goals/${id}`, { method: "DELETE" }),
  reorderGoals: (ids: number[]) => req<Goal[]>("/api/goals/order", { method: "PUT", body: JSON.stringify({ ids }) }),
  portfolio: () => req<PortfolioResponse>("/api/portfolio"),

  updateProfile: (profile: Omit<Profile, "photo" | "signature">) =>
    req<void>("/api/profile", { method: "PUT", body: JSON.stringify(profile) }),
  setPhoto: (data: string) =>
    req<void>("/api/photo", { method: "PUT", body: JSON.stringify({ data }) }),
  setSignature: (data: string) =>
    req<void>("/api/signature", { method: "PUT", body: JSON.stringify({ data }) }),
  reset: () => req<void>("/api/reset", { method: "POST", body: JSON.stringify({ confirm: "RESET" }) }),
};
