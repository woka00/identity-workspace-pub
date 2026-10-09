import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { AuthScreen } from "./presentation/auth/AuthScreen";
import { registrationTokenFromLocation, clearRegistrationFragment } from "./infrastructure/browser/registrationLink";
import { api, ApiError } from "./infrastructure/http/apiClient";
import {
  calculateNutritionGoals,
  caloriesFromMacros,
  DEFAULT_NUTRITION_CALCULATOR,
  DEFAULT_NUTRITION_GOALS,
} from "./application/nutritionGoals";
import {
  addDaysToDateKey,
  addMonthsToDateKey,
  dateKeyFromTimestamp,
  dateKeysInRange,
  formalTodayKey,
  localTodayKey,
  parseDateKey,
  startOfMonthDateKey,
  startOfWeekDateKey,
} from "./application/dateKeys";
import type {
  NutritionCalculatorActivity,
  NutritionCalculatorInput,
} from "./application/nutritionGoals";
import {
  AuthUser,
  WorkspacePreferences,
  WorkspaceFeature,
  BottomNavigationItem,
  CustomTracker,
  CustomTrackerEntry,
  CustomTrackerInput,
  FatSecretNutrition,
  FatSecretNutritionEntry,
  FatSecretFood,
  FatSecretFoodServing,
  FatSecretMeal,
  FatSecretStatus,
  Goal,
  GoalInput,
  GitHubTrackerState,
  LocalFood,
  NutritionGoals,
  PortfolioResponse,
  Profile,
  Task,
  TaskCategory,
  TaskInput,
  TaskRecurrenceType,
  TaskStatus,
  TrackerState,
  TrackerReminder,
  TrackerWaterEntry,
  TrackerWeightEntry,
} from "./domain/models";
import { downloadCalorieReport } from "./infrastructure/export/calorieReport";
import { decodeBarcodeFromImage, loadBarcodeImage, normalizeBarcodeValue } from "./infrastructure/browser/barcodeScanner";
import {
  registerDeviceNotifications,
  unregisterDeviceNotifications,
} from "./infrastructure/browser/deviceNotifications";
import type { DeviceNotificationState } from "./infrastructure/browser/deviceNotifications";
import {
  consumeFatSecretCallbackNotice,
} from "./infrastructure/browser/oauthCallbacks";
import type { IntegrationNotice } from "./infrastructure/browser/oauthCallbacks";
import { useVirtualKeyboardOpen } from "./presentation/hooks/useVirtualKeyboardOpen";
import { useTheme } from "./presentation/hooks/useTheme";
import ThemeSettings from "./presentation/profile/ThemeSettings";
import type { ThemePreferences } from "./domain/theme";
import type { ScannedNutrition } from "./infrastructure/browser/nutritionScanner";
import TaskDescriptionEditor from "./presentation/tasks/TaskDescriptionEditor";
import TaskScheduleInput from "./presentation/tasks/TaskScheduleInput";
import TimeTrackerNavIcon from "./presentation/time-tracker/TimeTrackerNavIcon";

import { workspaceNavigation } from "./application/workspace";
import WorkspaceOnboarding from "./presentation/onboarding/WorkspaceOnboarding";
import TimerIntroduction from "./presentation/time-tracker/TimerIntroduction";

const AdminFoodsPage = lazy(() => import("./presentation/nutrition/AdminFoodsPage"));
const BarcodeScannerSheet = lazy(() => import("./presentation/nutrition/BarcodeScannerSheet"));
const NutritionScannerSheet = lazy(() => import("./presentation/nutrition/NutritionScannerSheet"));
const ProjectProfileDialog = lazy(() => import("./presentation/profile/ProjectProfileDialog"));
const TimeTrackerPage = lazy(() => import("./presentation/time-tracker/TimeTrackerPage"));
const WorkPage = lazy(() => import("./presentation/work/WorkPage").then((module) => ({ default: module.WorkPage })));

type Page = "card" | "tasks" | "projects" | "tracker" | "calories" | "profile" | "food-admin";
type CalorieAccountingMode = "fatsecret" | "internal";
type ToastTone = "success" | "info" | "error";
type ProfileField = "name" | "surname" | "occupation" | "sex" | "dob";
type TrackerCardID = "calories" | "water" | "weight" | "github";
type TrackerSelectionID = TrackerCardID | `custom:${number}`;
type TrackerModal = "picker" | "reminders" | "statistics" | "calories" | "water" | "weight" | "github" | "custom-create" | "custom-edit" | "custom" | null;
type StatisticsPeriod = 7 | 30;
type GitHubPeriod = "day" | "week" | "year";
type NutritionGoalsMode = "quick" | "manual";

const DEFAULT_BOTTOM_NAVIGATION: BottomNavigationItem[] = ["card", "tasks", "tracker", "profile"];
const REQUIRED_BOTTOM_NAVIGATION = new Set<BottomNavigationItem>(["card", "profile"]);
const BOTTOM_NAVIGATION_OPTIONS: { id: BottomNavigationItem; label: string }[] = [
  { id: "card", label: "Карта" },
  { id: "tasks", label: "Задачи" },
  { id: "projects", label: "Работа" },
  { id: "tracker", label: "Трекер" },
  { id: "calories", label: "Калории" },
  { id: "profile", label: "Профиль" },
];

function normalizeBottomNavigation(items: unknown): BottomNavigationItem[] {
  if (!Array.isArray(items) || items.length < 2 || items.length > 6) return [...DEFAULT_BOTTOM_NAVIGATION];
  const allowed = new Set(BOTTOM_NAVIGATION_OPTIONS.map((item) => item.id));
  const normalized = items.filter((item): item is BottomNavigationItem => typeof item === "string" && allowed.has(item as BottomNavigationItem));
  const valid = normalized.length === items.length && new Set(normalized).size === normalized.length && [...REQUIRED_BOTTOM_NAVIGATION].every((item) => normalized.includes(item));
  return valid ? normalized : [...DEFAULT_BOTTOM_NAVIGATION];
}

interface TrackerSettings {
  version: 2;
  enabled: boolean;
  selected: TrackerSelectionID[];
  waterGoal: number;
  waterByDate: Record<string, number>;
  weightKg: number;
}

interface ToastMessage {
  id: number;
  title: string;
  detail?: string;
  tone: ToastTone;
}

type FatSecretNotice = IntegrationNotice;

const TRACKER_STORAGE_BASE = "avatar-id.trackers.v1";
const TRACKER_SERVER_IMPORT_BASE = "avatar-id.trackers.server-import.v1";
const TRACKER_LEGACY_CLAIM_KEY = "avatar-id.trackers.legacy-claimed-by";
const GITHUB_PERIOD_STORAGE_BASE = "identity-workspace.github-period.v1";
const CALORIE_MODE_STORAGE_BASE = "identity-workspace.calorie-mode.v1";
const NUTRITION_GOALS_PROMPT_STORAGE_BASE = "identity-workspace.nutrition-goals-prompt.v1";
const trackerStorageKey = (userID: number) => `${TRACKER_STORAGE_BASE}.user-${userID}`;
const trackerServerImportKey = (userID: number) => `${TRACKER_SERVER_IMPORT_BASE}.user-${userID}`;
const githubPeriodStorageKey = (userID: number) => `${GITHUB_PERIOD_STORAGE_BASE}.user-${userID}`;
const calorieModeStorageKey = (userID: number) => `${CALORIE_MODE_STORAGE_BASE}.user-${userID}`;
const nutritionGoalsPromptStorageKey = (userID: number) => `${NUTRITION_GOALS_PROMPT_STORAGE_BASE}.user-${userID}`;

function loadCalorieMode(userID: number): CalorieAccountingMode | null {
  try {
    const value = window.localStorage.getItem(calorieModeStorageKey(userID));
    return value === "fatsecret" || value === "internal" ? value : null;
  } catch {
    return null;
  }
}
const MAX_TRACKER_CARDS = 6;
const WATER_GLASS_ML = 250;
const CARD_EXPIRY_DATE = "13.07.2106";
const GITHUB_TRACKER_WEEKS = 53;
const GITHUB_TRACKER_CARD_WEEKS = 8;
const GITHUB_PERIODS: Array<{ value: GitHubPeriod; label: string; caption: string }> = [
  { value: "day", label: "День", caption: "коммитов сегодня" },
  { value: "week", label: "Неделя", caption: "коммитов за неделю" },
  { value: "year", label: "Год", caption: "коммитов за год" },
];
const FATSECRET_MEALS: Array<{ id: FatSecretMeal; apiName: string; label: string }> = [
  { id: "breakfast", apiName: "Breakfast", label: "Завтрак" },
  { id: "lunch", apiName: "Lunch", label: "Обед" },
  { id: "dinner", apiName: "Dinner", label: "Ужин" },
  { id: "other", apiName: "Other", label: "Перекусы" },
];
const TRACKER_CARD_ORDER: TrackerCardID[] = ["calories", "water", "weight", "github"];
const TRACKER_REMINDER_CARD_ORDER: TrackerCardID[] = ["calories", "water", "weight"];
const TRACKER_CARD_META: Record<TrackerCardID, { title: string; description: string }> = {
  calories: { title: "Калории", description: "Съедено за сегодня" },
  water: { title: "Вода", description: "Выпитые стаканы" },
  weight: { title: "Вес", description: "Последнее измерение" },
  github: { title: "GitHub активность", description: "Коммиты из профиля GitHub" },
};
const PROVIDED_TRACKER_ICONS = Array.from({ length: 36 }, (_, index) => {
  const number = String(index + 1).padStart(2, "0");
  return [`icon-${number}`, `/tracker-icons/icon-${number}.png`] as const;
});

const LEGACY_TRACKER_ICON_IDS = [
  "target", "home", "work", "book", "study", "fitness", "run", "bike", "walk", "water",
  "food", "sleep", "medicine", "heart", "money", "save", "shopping", "travel", "car", "plant",
  "pet", "music", "art", "camera", "code", "language", "habit", "clean", "family", "star",
] as const;

const AVATAR_TRACKER_ICONS = LEGACY_TRACKER_ICON_IDS.map((id) =>
  [id, `/tracker-icons/${id}.png`] as const,
);
const CUSTOM_TRACKER_ICONS = [...PROVIDED_TRACKER_ICONS, ...AVATAR_TRACKER_ICONS] as const;

function customTrackerIconSrc(icon: string) {
  if (icon === "sleep") return "/tracker-builtins/sleep.svg";
  return CUSTOM_TRACKER_ICONS.find(([id]) => id === icon)?.[1] ?? "/tracker-icons/icon-01.png";
}

function CustomTrackerIcon({ icon }: { icon: string }) {
  return <img src={customTrackerIconSrc(icon)} alt="" aria-hidden="true" draggable={false} />;
}

const PWA_SAFARI_GUIDE_STORAGE_KEY = "identity-workspace.pwa-safari-guide.v1";

function storageFlag(key: string) {
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

function persistStorageFlag(key: string) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(key, "1");
  } catch {
    // Одноразовое окно останется закрытым до перезагрузки текущей страницы.
  }
}

function shouldShowPWASafariGuide() {
  if (typeof window === "undefined" || storageFlag(PWA_SAFARI_GUIDE_STORAGE_KEY)) return false;
  const navigatorWithStandalone = window.navigator as Navigator & { standalone?: boolean };
  const standalone = window.matchMedia("(display-mode: standalone)").matches || navigatorWithStandalone.standalone === true;
  const phone = window.matchMedia("(max-width: 760px) and (pointer: coarse)").matches;
  const userAgent = window.navigator.userAgent;
  const iosPhone = /iPhone|iPod/i.test(userAgent);
  const safari = /Safari/i.test(userAgent) && !/CriOS|FxiOS|EdgiOS|OPiOS/i.test(userAgent);
  return phone && iosPhone && safari && !standalone;
}

function initialPage(): Page {
  if (typeof window === "undefined") return "card";
  if (new URL(window.location.href).pathname === "/admin/foods") return "food-admin";
  const url = new URL(window.location.href);
  if (url.searchParams.has("work_invite") || new URLSearchParams(url.hash.slice(1)).has("work_invite")) return "projects";
  const requested = url.searchParams.get("view");
  return requested === "tasks" || requested === "projects" || requested === "tracker" || requested === "calories" || requested === "profile" || requested === "card"
    ? requested
    : "card";
}

function defaultTrackerSettings(): TrackerSettings {
  return { version: 2, enabled: true, selected: ["calories", "weight"], waterGoal: 8, waterByDate: {}, weightKg: 92 };
}

function trackerInteger(value: unknown, minimum: number, maximum: number, fallback: number) {
  const number = Math.round(Number(value));
  return Number.isFinite(number) ? Math.max(minimum, Math.min(maximum, number)) : fallback;
}

function trackerDecimal(value: unknown, minimum: number, maximum: number, fallback: number) {
  const number = Number(String(value).replace(",", "."));
  if (!Number.isFinite(number)) return fallback;
  return Math.round(Math.max(minimum, Math.min(maximum, number)) * 10) / 10;
}

function formatTrackerWeight(value: number) {
  return value.toLocaleString("ru-RU", { minimumFractionDigits: 0, maximumFractionDigits: 1 });
}

function formatTrackerNumber(value: number) {
  return value.toLocaleString("ru-RU", { minimumFractionDigits: 0, maximumFractionDigits: 3 });
}

function activeTrackerLabel(count: number) {
  if (count === 1) return "активный";
  if (count >= 2 && count <= 4) return "активных";
  return "активных";
}

function formatProfileDateInput(value: string) {
  const digits = value.replace(/\D/g, "").slice(0, 8);
  if (digits.length <= 2) return digits;
  if (digits.length <= 4) return `${digits.slice(0, 2)}.${digits.slice(2)}`;
  return `${digits.slice(0, 2)}.${digits.slice(2, 4)}.${digits.slice(4)}`;
}

function githubUsernameIsValid(value: string) {
  const username = value.trim();
  return username.length <= 39 && !username.includes("--") && /^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$/.test(username);
}

function githubPeriodMeta(period: GitHubPeriod) {
  return GITHUB_PERIODS.find((item) => item.value === period) ?? GITHUB_PERIODS[1];
}

function loadGitHubPeriod(userID: number): GitHubPeriod {
  if (typeof window === "undefined") return "week";
  try {
    const period = window.localStorage.getItem(githubPeriodStorageKey(userID));
    return period === "day" || period === "week" || period === "year" ? period : "week";
  } catch {
    return "week";
  }
}

function persistGitHubPeriod(userID: number, period: GitHubPeriod) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(githubPeriodStorageKey(userID), period);
  } catch {
    // Выбранный период продолжает работать до закрытия текущей вкладки.
  }
}

function githubContributionCount(state: GitHubTrackerState | null, period: GitHubPeriod, currentDate: string) {
  if (!state) return 0;
  if (period === "day") {
    if (Number.isFinite(state.todayContributions)) return state.todayContributions;
    return state.days.find((day) => day.date === currentDate)?.count ?? 0;
  }
  if (period === "week") {
    if (Number.isFinite(state.weekContributions)) return state.weekContributions;
    const weekStart = startOfWeekDateKey(currentDate);
    return state.days.filter((day) => day.date >= weekStart && day.date <= currentDate).reduce((sum, day) => sum + day.count, 0);
  }
  if (Number.isFinite(state.yearContributions)) return state.yearContributions;
  const yearStart = addDaysToDateKey(currentDate, -364);
  return state.days.filter((day) => day.date >= yearStart && day.date <= currentDate).reduce((sum, day) => sum + day.count, 0);
}

function formatNutrition(value: number, maximumFractionDigits = 1) {
  return value.toLocaleString("ru-RU", { minimumFractionDigits: 0, maximumFractionDigits });
}

function nutritionEntryServingLabel(entry: FatSecretNutritionEntry) {
  const units = formatNutrition(entry.numberOfUnits);
  const description = entry.description.trim();
  if (entry.servingId === "g" || description === "г") return `${units} г`;
  if (entry.servingId === "ml" || description === "мл") return `${units} мл`;
  if (!description) return entry.numberOfUnits === 1 ? "1 порция" : `${units} × порция`;
  if (entry.servingId === "portion") return entry.numberOfUnits === 1 ? `1 ${description}` : `${units} × ${description}`;
  return entry.numberOfUnits === 1 ? description : `${units} × ${description}`;
}

function loadTrackerSettings(userID: number): TrackerSettings {
  const defaults = defaultTrackerSettings();
  if (typeof window === "undefined") return defaults;
  try {
    const scopedKey = trackerStorageKey(userID);
    let raw = window.localStorage.getItem(scopedKey);
    if (raw === null) {
      const legacyOwner = window.localStorage.getItem(TRACKER_LEGACY_CLAIM_KEY);
      const legacyRaw = window.localStorage.getItem(TRACKER_STORAGE_BASE);
      if (legacyRaw && (legacyOwner === null || legacyOwner === String(userID))) {
        raw = legacyRaw;
        window.localStorage.setItem(TRACKER_LEGACY_CLAIM_KEY, String(userID));
        window.localStorage.setItem(scopedKey, legacyRaw);
      }
    }
    const parsed = JSON.parse(raw ?? "null") as Partial<TrackerSettings> | null;
    if (!parsed || typeof parsed !== "object") return defaults;
    const selected = Array.isArray(parsed.selected)
      ? parsed.selected
          .filter((id): id is TrackerSelectionID => typeof id === "string" && (TRACKER_CARD_ORDER.includes(id as TrackerCardID) || /^custom:\d+$/.test(id)))
          .slice(0, MAX_TRACKER_CARDS)
      : defaults.selected;
    const waterByDate: Record<string, number> = {};
    if (parsed.waterByDate && typeof parsed.waterByDate === "object") {
      Object.entries(parsed.waterByDate).forEach(([date, value]) => {
        if (/^\d{4}-\d{2}-\d{2}$/.test(date)) waterByDate[date] = trackerInteger(value, 0, 99, 0);
      });
    }
    return {
      version: 2,
      enabled: typeof parsed.enabled === "boolean" ? parsed.enabled : defaults.enabled,
      selected,
      waterGoal: trackerInteger(parsed.waterGoal, 1, 30, defaults.waterGoal),
      waterByDate,
      weightKg: trackerDecimal(parsed.weightKg, 20, 500, defaults.weightKg),
    };
  } catch {
    return defaults;
  }
}

function loadLegacyTrackerSnapshot(userID: number) {
  const settings = loadTrackerSettings(userID);
  if (typeof window === "undefined") return { settings, hasStoredWeight: false };
  try {
    const parsed = JSON.parse(window.localStorage.getItem(trackerStorageKey(userID)) ?? "null") as Partial<TrackerSettings> | null;
    const weight = Number(String(parsed?.weightKg ?? "").replace(",", "."));
    return {
      settings,
      hasStoredWeight: Number.isFinite(weight) && weight >= 20 && weight <= 500,
    };
  } catch {
    return { settings, hasStoredWeight: false };
  }
}

function persistTrackerSelection(userID: number, selected: TrackerSelectionID[]) {
  if (typeof window === "undefined") return;
  try {
    const parsed = JSON.parse(window.localStorage.getItem(trackerStorageKey(userID)) ?? "null") as Record<string, unknown> | null;
    const previous = parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
    window.localStorage.setItem(trackerStorageKey(userID), JSON.stringify({ ...previous, version: 2, selected }));
  } catch {
    // Выбор карточек продолжает работать в текущей сессии без localStorage.
  }
}

function persistTrackerVisibility(userID: number, enabled: boolean) {
  if (typeof window === "undefined") return;
  try {
    const parsed = JSON.parse(window.localStorage.getItem(trackerStorageKey(userID)) ?? "null") as Record<string, unknown> | null;
    const previous = parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
    window.localStorage.setItem(trackerStorageKey(userID), JSON.stringify({ ...previous, version: 2, enabled }));
  } catch {
    // Настройка продолжает работать в текущей сессии без localStorage.
  }
}

function trackerStateWithWater(state: TrackerState, entry: TrackerWaterEntry): TrackerState {
  const waterHistory = [...state.waterHistory.filter((item) => item.date !== entry.date), entry]
    .sort((left, right) => left.date.localeCompare(right.date));
  return { ...state, waterGoal: entry.goalGlasses, waterHistory };
}

function trackerStateWithWeight(state: TrackerState, entry: TrackerWeightEntry): TrackerState {
  const weightHistory = [...state.weightHistory.filter((item) => item.date !== entry.date), entry]
    .sort((left, right) => left.date.localeCompare(right.date));
  return {
    ...state,
    currentWeightKg: weightHistory.length > 0 ? weightHistory[weightHistory.length - 1].weightKg : null,
    weightHistory,
  };
}

function trackerStateWithCustom(state: TrackerState, tracker: CustomTracker, date: string): TrackerState {
  const entry: CustomTrackerEntry = {
    trackerId: tracker.id,
    date,
    value: tracker.currentValue,
    targetValue: tracker.targetValue,
    updatedAt: tracker.updatedAt,
  };
  const customHistory = [...(state.customHistory ?? []).filter((item) => item.trackerId !== tracker.id || item.date !== date), entry]
    .sort((left, right) => left.date.localeCompare(right.date) || left.trackerId - right.trackerId);
  return {
    ...state,
    customTrackers: state.customTrackers.map((item) => item.id === tracker.id ? tracker : item),
    customHistory,
  };
}

const CARD_PHOTO_SIZE = 1200;
const CARD_PHOTO_PNG_LIMIT = 4_200_000;

function canvasToDataURL(canvas: HTMLCanvasElement, type: string, quality?: number) {
  return new Promise<string>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error("не удалось сохранить обработанное фото"));
        return;
      }
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.onerror = () => reject(new Error("не удалось прочитать обработанное фото"));
      reader.readAsDataURL(blob);
    }, type, quality);
  });
}

function applyCardPhotoEffect(ctx: CanvasRenderingContext2D, width: number, height: number) {
  const image = ctx.getImageData(0, 0, width, height);
  const { data } = image;
  const grayscale = 1;
  const color = 1 - grayscale;
  const contrast = 1.1;
  const brightness = 1.04;
  const clamp = (value: number) => Math.max(0, Math.min(255, Math.round(value)));

  for (let i = 0; i < data.length; i += 4) {
    if (data[i + 3] === 0) continue;
    const gray = data[i] * 0.2126 + data[i + 1] * 0.7152 + data[i + 2] * 0.0722;
    data[i] = clamp(((data[i] * color + gray * grayscale - 128) * contrast + 128) * brightness);
    data[i + 1] = clamp(((data[i + 1] * color + gray * grayscale - 128) * contrast + 128) * brightness);
    data[i + 2] = clamp(((data[i + 2] * color + gray * grayscale - 128) * contrast + 128) * brightness);
  }

  ctx.putImageData(image, 0, 0);
}

const emptyPortfolio: PortfolioResponse = { pinned: [], active: [], completed: [] };


export default function App() {
  const [registrationToken, setRegistrationToken] = useState(registrationTokenFromLocation);
  const [registrationActive, setRegistrationActive] = useState(Boolean(registrationToken));
  useEffect(() => {
    clearRegistrationFragment();
    const receiveLink = () => {
      const token = registrationTokenFromLocation();
      if (token) { setRegistrationToken(token); setRegistrationActive(true); setPWASafariGuideOpen(false); clearRegistrationFragment(); }
    };
    window.addEventListener("hashchange", receiveLink);
    return () => window.removeEventListener("hashchange", receiveLink);
  }, []);
  const [user, setUser] = useState<AuthUser | null>(null);
  const [checking, setChecking] = useState(true);
  const [serverError, setServerError] = useState("");
  const [pwaSafariGuideOpen, setPWASafariGuideOpen] = useState(() => !registrationToken && shouldShowPWASafariGuide());

  const checkSession = useCallback(async () => {
    setChecking(true);
    setServerError("");
    try {
      const session = await api.authSession();
      setUser(session.user);
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        setUser(null);
      } else {
        setServerError(cause instanceof Error ? cause.message : "сервер недоступен");
        setUser(null);
      }
    } finally {
      setChecking(false);
    }
  }, []);

  useEffect(() => {
    void checkSession();
    const unauthorized = () => setUser(null);
    window.addEventListener("avatar-id:unauthorized", unauthorized);
    return () => window.removeEventListener("avatar-id:unauthorized", unauthorized);
  }, [checkSession]);

  async function logout() {
    try {
      await unregisterDeviceNotifications();
      await api.logout();
    } finally {
      setUser(null);
    }
  }

  function finishPWASafariGuide() {
    persistStorageFlag(PWA_SAFARI_GUIDE_STORAGE_KEY);
    setPWASafariGuideOpen(false);
  }

  if (pwaSafariGuideOpen && !registrationActive) return <PWASafariGuide onDone={finishPWASafariGuide} />;
  if (checking) return <div className="boot">ПРОВЕРКА ДОСТУПА<span className="blink">▌</span></div>;
  if (!user || registrationActive) {
    return <AuthScreen registrationToken={registrationToken} onDiscardToken={() => setRegistrationToken("")} serverError={serverError} onAuthenticated={(nextUser) => { setRegistrationToken(""); setRegistrationActive(false); setServerError(""); setUser(nextUser); }} onRetry={checkSession} />;
  }
  return <AuthenticatedApp key={user.id} user={user} onLogout={logout} />;
}

function OnboardingScreenshot({ src, step, alt }: { src: string; step: number; alt: string }) {
  const [available, setAvailable] = useState(true);
  return (
    <div className="onboardingScreenshot">
      {available
        ? <img src={src} alt={alt} draggable={false} onError={() => setAvailable(false)} />
        : <div className="onboardingScreenshotPlaceholder" aria-hidden="true"><span>{step}</span><small>Скриншот шага</small></div>}
    </div>
  );
}

function PWASafariGuide({ onDone }: { onDone: () => void }) {
  useEffect(() => {
    persistStorageFlag(PWA_SAFARI_GUIDE_STORAGE_KEY);
  }, []);

  const steps = [
    { title: "Откройте меню Safari", text: "Нажмите кнопку с тремя точками в правом нижнем углу Safari.", image: "/onboarding/pwa-safari/photo_1_2026-08-19_20-00-33.jpg" },
    { title: "Нажмите «Поделиться»", text: "В открывшемся меню Safari выберите действие «Поделиться».", image: "/onboarding/pwa-safari/photo_2_2026-08-19_20-00-33.jpg" },
    { title: "Покажите остальные действия", text: "В меню «Поделиться» нажмите «Показать больше».", image: "/onboarding/pwa-safari/photo_3_2026-08-19_20-00-33.jpg" },
    { title: "Добавьте на экран «Домой»", text: "В расширенном списке выберите «Добавить на экран “Домой”».", image: "/onboarding/pwa-safari/photo_4_2026-08-19_20-00-33.jpg" },
    { title: "Подтвердите установку", text: "Оставьте «Открыть как веб-приложение» включённым и нажмите «Добавить». Identity Workspace появится на домашнем экране.", image: "/onboarding/pwa-safari/photo_5_2026-08-19_20-00-33.jpg" },
  ];

  return (
    <main className="onboardingPage" role="dialog" aria-modal="true" aria-labelledby="pwa-guide-title">
      <section className="onboardingPanel">
        <header className="onboardingHero">
          <img src="/identity-workspace-icon-192-v2.png" alt="" aria-hidden="true" draggable={false} />
          <div><span>Установка на iPhone</span><h1 id="pwa-guide-title">Добавьте Identity Workspace на экран «Домой»</h1><p>Так приложение будет открываться без панели Safari, а Web Push сможет работать в поддерживаемых версиях iOS.</p></div>
        </header>
        <div className="onboardingSteps">
          {steps.map((step, index) => (
            <article className="onboardingStep" key={step.title}>
              <OnboardingScreenshot src={step.image} step={index + 1} alt={step.title} />
              <div><span>{String(index + 1).padStart(2, "0")}</span><strong>{step.title}</strong><p>{step.text}</p></div>
            </article>
          ))}
        </div>
        <button type="button" className="primaryButton onboardingDone" onClick={onDone}>Продолжить в браузере</button>
      </section>
    </main>
  );
}

function AuthenticatedApp({ user, onLogout }: { user: AuthUser; onLogout: () => Promise<void> }) {
  const [themePreferences, changeTheme] = useTheme(user.id);
  const [page, setPage] = useState<Page>(() => {
    const requested = initialPage();
    return requested === "food-admin" && !user.isAdmin ? "profile" : requested;
  });
  const [workspace, setWorkspace] = useState<WorkspacePreferences | null>(null);
  const [workspaceEditing, setWorkspaceEditing] = useState(false);
  const [timerIntroOpen, setTimerIntroOpen] = useState(false);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [currentDate, setCurrentDate] = useState("");
  const [savedTasks, setTasks] = useState<Task[]>([]);
  const [taskStatusOverrides, setTaskStatusOverrides] = useState<Record<number, TaskStatus>>({});
  const taskStatusRequests = useRef(new Map<number, Promise<void>>());
  const tasks = useMemo(() => savedTasks.map((task) => {
    const status = taskStatusOverrides[task.id];
    return status ? { ...task, status, completedAt: status === "done" ? (task.completedAt || new Date().toISOString()) : "" } : task;
  }), [savedTasks, taskStatusOverrides]);
  const [goals, setGoals] = useState<Goal[]>([]);
  const [portfolio, setPortfolio] = useState<PortfolioResponse>(emptyPortfolio);
  const [profileEditing, setProfileEditing] = useState<ProfileField | null>(null);
  const [signatureEditing, setSignatureEditing] = useState(false);
  const [taskEditing, setTaskEditing] = useState<Task | null>(null);
  const [goalEditing, setGoalEditing] = useState<Goal | "new" | null>(null);
  const [taskOrderBusy, setTaskOrderBusy] = useState(false);
  const [showTimeline, setShowTimeline] = useState(false);
  const [trackersEnabled, setTrackersEnabled] = useState(() => loadTrackerSettings(user.id).enabled);
  const [calorieMode, setCalorieMode] = useState<CalorieAccountingMode | null>(() => loadCalorieMode(user.id));
  const [calorieModePickerOpen, setCalorieModePickerOpen] = useState(false);
  const [bottomNavigation, setBottomNavigation] = useState<BottomNavigationItem[]>([...DEFAULT_BOTTOM_NAVIGATION]);
  const [bottomNavigationSaving, setBottomNavigationSaving] = useState(false);
  const visibleNavigation = useMemo(() => {
    const items = workspaceNavigation(workspace, bottomNavigation);
    if (workspace?.features.includes("projects") && !items.includes("projects")) items.splice(Math.max(1, items.length - 1), 0, "projects");
    return items;
  }, [workspace, bottomNavigation]);
  const featureEnabled = (feature: WorkspaceFeature) => !workspace?.onboardingCompleted || workspace.features.includes(feature);
  const workspaceIntroOpen = !!workspace && (!workspace.onboardingCompleted || workspaceEditing);
  const showTimerIntro = !!workspace?.onboardingCompleted && !workspaceIntroOpen && page === "tracker" && (!workspace.timerIntroSeen || timerIntroOpen);

  useEffect(() => {
    if (!workspace?.onboardingCompleted) return;
    if ((page === "tasks" || page === "tracker") && !visibleNavigation.includes(page)) setPage("card");
    if (page === "projects" && !workspace.features.includes("projects")) setPage("card");
    if (page === "calories" && !workspace.features.includes("calories")) setPage("card");
  }, [workspace, visibleNavigation, page]);

  async function saveWorkspace(features: WorkspaceFeature[], navigation: BottomNavigationItem[]) {
    if (!workspace) return;
    const next = { ...workspace, features, onboardingCompleted: true };
    const previousNavigation = bottomNavigation;
    await api.updateBottomNavigation(navigation);
    try {
      await api.updateWorkspacePreferences(next);
    } catch (cause) {
      void api.updateBottomNavigation(previousNavigation).catch(() => undefined);
      throw cause;
    }
    setWorkspace(next);
    setBottomNavigation(navigation);
    setTrackerVisibility(features.includes("widgets") || features.includes("calories"));
    setWorkspaceEditing(false);
  }

  async function finishTimerIntro() {
    if (!workspace) return;
    if (!workspace.timerIntroSeen) {
      const next = { ...workspace, timerIntroSeen: true };
      await api.updateWorkspacePreferences(next);
      setWorkspace(next);
    }
    setTimerIntroOpen(false);
  }
  const temporaryTaskID = useRef(-1);
  const [photoProcessing, setPhotoProcessing] = useState(false);
  const [photoProgress, setPhotoProgress] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  const toastID = useRef(0);
  const fileRef = useRef<HTMLInputElement>(null);

  function chooseCalorieMode(mode: CalorieAccountingMode) {
    setCalorieMode(mode);
    setCalorieModePickerOpen(false);
    try { window.localStorage.setItem(calorieModeStorageKey(user.id), mode); } catch { /* The choice remains active for this session. */ }
    setPage("calories");
  }

  function openCalories() {
    if (!calorieMode) {
      setCalorieModePickerOpen(true);
      return;
    }
    setPage("calories");
  }

  function openBottomNavigationItem(item: BottomNavigationItem) {
    if (item === "calories") {
      openCalories();
      return;
    }
    setPage(item);
  }

  const addToast = useCallback((title: string, detail?: string, tone: ToastTone = "info") => {
    // Routine actions update the interface silently. Only errors may interrupt the user.
    if (tone !== "error") return;
    const id = ++toastID.current;
    setToasts((items) => [...items.slice(-3), { id, title, detail, tone }]);
    window.setTimeout(
      () => setToasts((items) => items.filter((item) => item.id !== id)),
      4200,
    );
  }, []);

  const flashError = useCallback((cause: unknown) => {
    const message = cause instanceof Error ? cause.message : String(cause);
    setError(message);
    addToast("Ошибка", message, "error");
    window.setTimeout(() => setError(null), 4500);
  }, [addToast]);

  const load = useCallback(async () => {
    try {
      const [state, loadedTasks, loadedGoals, loadedPortfolio] = await Promise.all([
        api.state(false),
        api.tasks(),
        api.goals(),
        api.portfolio(),
      ]);
      setWorkspace(state.workspacePreferences);
      setProfile(state.profile);
      setCurrentDate(state.currentDate);
      setTasks(loadedTasks);
      setGoals(loadedGoals);
      setPortfolio(loadedPortfolio);
      setBottomNavigation(normalizeBottomNavigation(state.bottomNavigation));
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "сервер недоступен");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const today = formalTodayKey(currentDate || localTodayKey());
    const activeCount = tasks.filter((task) => task.status !== "done" && task.dueDate === today).length;
    document.title = `${activeCount} задач · identity workspace`;
  }, [tasks, currentDate]);

  useEffect(() => {
    const modalOpen = profileEditing !== null || signatureEditing || taskEditing !== null || goalEditing !== null || showTimeline;
    if (!modalOpen) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [profileEditing, signatureEditing, taskEditing, goalEditing, showTimeline]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (workspaceIntroOpen || showTimerIntro) return;
      if (event.key === "Escape") {
        setProfileEditing(null);
        setSignatureEditing(false);
        setTaskEditing(null);
        setGoalEditing(null);
        setShowTimeline(false);
        return;
      }
      const target = event.target as HTMLElement | null;
      if (
        event.metaKey || event.ctrlKey || event.altKey || target?.isContentEditable ||
        ["INPUT", "TEXTAREA", "SELECT"].includes(target?.tagName ?? "")
      ) return;
      const shortcutIndex = Number(event.key) - 1;
      if (shortcutIndex >= 0 && shortcutIndex < visibleNavigation.length) {
        openBottomNavigationItem(visibleNavigation[shortcutIndex]);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [visibleNavigation, calorieMode, page, workspaceIntroOpen, showTimerIntro]);

  async function refreshPortfolioAndGoals() {
    const [loadedGoals, loadedPortfolio] = await Promise.all([api.goals(), api.portfolio()]);
    setGoals(loadedGoals);
    setPortfolio(loadedPortfolio);
  }

  async function createTask(input: TaskInput) {
    const optimisticID = temporaryTaskID.current--;
    const optimisticTask: Task = {
      id: optimisticID,
      ...input,
      reminderSentAt: "",
      createdAt: new Date().toISOString(),
      completedAt: "",
      sortOrder: Number.MAX_SAFE_INTEGER,
    };
    let serverTask: Task | null = null;
    let optimisticVisible = false;
    let revealTimer: number | undefined;
    const revealTask = new Promise<void>((resolve) => {
      revealTimer = window.setTimeout(() => {
        optimisticVisible = true;
        const visibleTask = serverTask ?? optimisticTask;
        setTasks((items) => [visibleTask, ...items.filter((item) => item.id !== visibleTask.id && item.id !== optimisticID)]);
        resolve();
      }, 100);
    });

    try {
      serverTask = await api.createTask(input);
      await revealTask;
      if (optimisticVisible) {
        setTasks((items) => {
          if (items.some((item) => item.id === optimisticID)) {
            return items
              .filter((item) => item.id !== serverTask!.id)
              .map((item) => item.id === optimisticID ? serverTask! : item);
          }
          return items.some((item) => item.id === serverTask!.id) ? items : [serverTask!, ...items];
        });
      }
      if (input.projectIds?.length) void refreshPortfolioAndGoals().catch(flashError);
      return true;
    } catch (cause) {
      if (revealTimer !== undefined) window.clearTimeout(revealTimer);
      if (optimisticVisible) setTasks((items) => items.filter((item) => item.id !== optimisticID));
      flashError(cause);
      return false;
    }
  }

  async function saveTask(input: TaskInput) {
    if (!taskEditing) return;
    const previousTask = taskEditing;
    const optimisticTask: Task = {
      ...previousTask,
      ...input,
      completedAt: input.status === "done" ? (previousTask.completedAt || new Date().toISOString()) : "",
      reminderSentAt: input.reminderAt === previousTask.reminderAt ? previousTask.reminderSentAt : "",
    };

    // Редактор закрывается сразу, а серверное сохранение завершается в фоне.
    setTaskEditing(null);
    setTasks((items) => items.map((item) => item.id === previousTask.id ? optimisticTask : item));
    try {
      const task = await api.updateTask(previousTask.id, input);
      if (input.status === "done" && input.recurrenceType) {
        setTasks(await api.tasks());
      } else {
        setTasks((items) => items.map((item) => item.id === task.id ? task : item));
      }
    } catch (cause) {
      setTasks((items) => items.map((item) => item.id === previousTask.id ? previousTask : item));
      setTaskEditing(previousTask);
      flashError(cause);
    }
  }

  async function moveTaskToToday(task: Task, date: string) {
    try {
      const updated = await api.updateTask(task.id, {
        title: task.title,
        description: task.description,
        category: task.category,
        status: task.status,
        dueDate: date,
        dueTime: task.dueTime,
        reminderAt: task.reminderAt,
        priority: task.priority,
        isMilestone: task.isMilestone,
        recurrenceType: task.recurrenceType,
        recurrenceInterval: task.recurrenceInterval,
        recurrenceEndDate: task.recurrenceEndDate,
        recurrenceWeekdays: task.recurrenceWeekdays,
      });
      setTasks((items) => items.map((item) => item.id === updated.id ? updated : item));
    } catch (cause) {
      flashError(cause);
    }
  }

  function setTrackerVisibility(enabled: boolean) {
    setTrackersEnabled(enabled);
    persistTrackerVisibility(user.id, enabled);
  }

  async function saveBottomNavigation(items: BottomNavigationItem[]) {
    if (bottomNavigationSaving) return;
    const previous = bottomNavigation;
    setBottomNavigation(items);
    setBottomNavigationSaving(true);
    try {
      await api.updateBottomNavigation(items);
    } catch (cause) {
      setBottomNavigation(previous);
      flashError(cause);
    } finally {
      setBottomNavigationSaving(false);
    }
  }

  async function setTaskStatus(task: Task, status: TaskStatus) {
    setTaskStatusOverrides((current) => ({ ...current, [task.id]: status }));
    const previous = taskStatusRequests.current.get(task.id);
    const request = (async () => {
      // Keep rapid toggles ordered while displaying the latest choice immediately.
      await previous;
      try {
        const updated = status === "done"
          ? await api.completeTask(task.id)
          : await api.uncompleteTask(task.id);
        setTasks((items) => items.map((item) => item.id === updated.id ? updated : item));
        if (status === "done" && task.recurrenceType) setTasks(await api.tasks());
      } catch (cause) {
        flashError(cause);
      }
    })();
    taskStatusRequests.current.set(task.id, request);
    await request;
    if (taskStatusRequests.current.get(task.id) === request) {
      taskStatusRequests.current.delete(task.id);
      // Removing the pending choice also restores confirmed state on a save failure.
      setTaskStatusOverrides((current) => {
        const next = { ...current };
        delete next[task.id];
        return next;
      });
    }
  }

  async function deleteTask(task: Task) {
    if (!window.confirm(`Удалить задачу «${task.title}»?`)) return;
    try {
      await api.deleteTask(task.id);
      setTasks((items) => items.filter((item) => item.id !== task.id));
      addToast("Задача удалена", task.title, "info");
    } catch (cause) {
      flashError(cause);
    }
  }

  async function moveTaskInDay(task: Task, otherTask: Task) {
    if (taskOrderBusy) return;
    setTaskOrderBusy(true);
    setTasks((items) => items.map((item) => {
      if (item.id === task.id) return { ...item, sortOrder: otherTask.sortOrder };
      if (item.id === otherTask.id) return { ...item, sortOrder: task.sortOrder };
      return item;
    }));
    try {
      setTasks(await api.swapTaskOrder(task.id, otherTask.id));
    } catch (cause) {
      setTasks((items) => items.map((item) => {
        if (item.id === task.id) return { ...item, sortOrder: task.sortOrder };
        if (item.id === otherTask.id) return { ...item, sortOrder: otherTask.sortOrder };
        return item;
      }));
      flashError(cause);
    } finally {
      setTaskOrderBusy(false);
    }
  }

  async function saveGoal(input: GoalInput) {
    try {
      const saved = goalEditing === "new"
        ? await api.createGoal(input)
        : await api.updateGoal((goalEditing as Goal).id, input);
      setGoalEditing(null);
      await refreshPortfolioAndGoals();
      addToast(saved.completed ? "Проект завершён" : "Проект сохранён", saved.title, "success");
    } catch (cause) {
      flashError(cause);
    }
  }

  async function deleteGoal(goal: Goal) {
    if (!window.confirm(`Удалить проект «${goal.title}»?`)) return;
    try {
      await api.deleteGoal(goal.id);
      setGoalEditing(null);
      await refreshPortfolioAndGoals();
      addToast("Проект удалён", goal.title, "info");
    } catch (cause) {
      flashError(cause);
    }
  }

  async function onPhotoPick(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;

    setPhotoProcessing(true);
    setPhotoProgress(0);
    let photoURL = "";
    try {
      photoURL = URL.createObjectURL(file);
      const image = await new Promise<HTMLImageElement>((resolve, reject) => {
        const result = new Image();
        result.onload = () => resolve(result);
        result.onerror = () => reject(new Error("не удалось прочитать фото"));
        result.src = photoURL;
      });
      setPhotoProgress(35);

      const width = CARD_PHOTO_SIZE;
      const height = CARD_PHOTO_SIZE;
      const canvas = document.createElement("canvas");
      canvas.width = width;
      canvas.height = height;
      const ctx = canvas.getContext("2d");
      if (!ctx) throw new Error("canvas недоступен");
      ctx.imageSmoothingEnabled = true;
      ctx.imageSmoothingQuality = "high";
      const scale = Math.min(width / image.width, height / image.height);
      const drawWidth = image.width * scale;
      const drawHeight = image.height * scale;
      ctx.drawImage(image, (width - drawWidth) / 2, height - drawHeight, drawWidth, drawHeight);
      applyCardPhotoEffect(ctx, width, height);
      setPhotoProgress(80);

      // Сохраняем исходный фон и всю композицию снимка. Для слишком большого
      // PNG используем WebP без заметной потери качества.
      let data = await canvasToDataURL(canvas, "image/png");
      if (data.length > CARD_PHOTO_PNG_LIMIT) {
        data = await canvasToDataURL(canvas, "image/webp", 0.98);
      }
      await api.setPhoto(data);
      setPhotoProgress(100);
      await load();
      addToast("Фото обновлено", "Чёрно-белый стиль карты применён.", "success");
    } catch (cause) {
      flashError(new Error(`не удалось обработать фото: ${cause instanceof Error ? cause.message : String(cause)}`));
    } finally {
      if (photoURL) URL.revokeObjectURL(photoURL);
      setPhotoProcessing(false);
      setPhotoProgress(null);
    }
  }

  async function resetAll() {
    try {
      await api.reset();
      await load();
      addToast("Рабочие данные сброшены", "Профиль и фотография сохранены.", "info");
      return true;
    } catch (cause) {
      flashError(cause);
      return false;
    }
  }

  if (error && !profile) {
    return (
      <div className="boot">
        СЕРВЕР НЕДОСТУПЕН
        <div className="bootSub">{error}</div>
        <button className="primaryButton" onClick={() => void load()}>Повторить</button>
      </div>
    );
  }
  if (!profile) return <div className="boot">ЗАГРУЗКА ДОКУМЕНТА<span className="blink">▌</span></div>;

  const todayKey = formalTodayKey(currentDate || localTodayKey());
  const activeTasks = tasks.filter((task) => task.status !== "done" && task.dueDate === todayKey).length;
  return (
    <div className="root">
      <main className={`sheet page-${page}`}>
        <nav className="tabs workspaceTabs" style={{ "--navigation-count": visibleNavigation.length } as CSSProperties} aria-label="Основные разделы">
          <div className="desktopNavBrand" aria-hidden="true">
            <img src="/identity-workspace-icon-192-v2.png" alt="" draggable={false} />
            <span>identity workspace</span>
          </div>
          {visibleNavigation.map((item) => (
            <Tab
              key={item}
              active={page === item || (item === "profile" && page === "food-admin")}
              icon={<BottomNavigationIcon item={item} />}
              label={bottomNavigationLabel(item)}
              meta={item === "card" ? "ID" : item === "tasks" ? String(activeTasks) : item === "profile" ? profile.name.slice(0, 1).toUpperCase() || "ID" : undefined}
              onClick={() => openBottomNavigationItem(item)}
            />
          ))}
        </nav>

        {page === "card" && (
          <>
            <IDCard
              profile={profile}
              pinned={portfolio.pinned}
              photoProcessing={photoProcessing}
              photoProgress={photoProgress}
              onEdit={(field) => setProfileEditing(field)}
              onSignatureClick={() => setSignatureEditing(true)}
              onPhotoClick={() => fileRef.current?.click()}
              onTimeline={() => setShowTimeline(true)}
            />
            <input ref={fileRef} type="file" accept="image/*" hidden onChange={onPhotoPick} />

            {(featureEnabled("widgets") || featureEnabled("calories")) && trackersEnabled && <TrackerPreview currentDate={currentDate} userID={user.id} calorieMode={calorieMode} onOpenCalories={featureEnabled("calories") ? openCalories : undefined} widgetsEnabled={featureEnabled("widgets")} caloriesEnabled={featureEnabled("calories")} />}

            {featureEnabled("projects") && <section className="doc portfolioDigest">
              <header className="docTitle">
                <button type="button" className="portfolioTitleLink" onClick={() => setPage("projects")}><span>Портфолио</span><span aria-hidden="true">›</span></button>
                <span className="docTitleEn">{portfolio.active.length} в работе · {portfolio.completed.length} завершено</span>
              </header>
              {portfolio.active.length === 0 && portfolio.completed.length === 0 ? (
                <EmptyState title="Портфолио пока пусто" text="Создайте первый проект — он появится здесь." />
              ) : (
                <div className="digestRows">
                  {[...portfolio.active, ...portfolio.completed].map((goal) => (
                    <div className={`digestRow ${goal.completed ? "" : "digestRowActive"}`} key={goal.id}>
                      <time>{goal.completed ? formatDate(goal.completedAt) : "В работе"}</time>
                      <strong>{goal.title}</strong>
                      <span>{goal.summary || goal.description || (goal.completed ? "Завершённый проект" : `${goal.completionPct}% выполнено`)}</span>
                      <button type="button" className="digestRowAction" onClick={() => setShowTimeline(true)} aria-label={`Открыть портфолио: ${goal.title}`}>→</button>
                    </div>
                  ))}
                </div>
              )}
              <button className="textButton timelineOpen" onClick={() => setShowTimeline(true)}>
                Открыть полный таймлайн →
              </button>
            </section>}
          </>
        )}

        {page === "tasks" && featureEnabled("tasks") && (
          <TasksPage
            tasks={tasks}
            projects={goals}
            currentDate={currentDate}
            onCreate={createTask}
            onEdit={setTaskEditing}
            onStatus={setTaskStatus}
            onMoveToToday={moveTaskToToday}
            onMoveInDay={moveTaskInDay}
            orderBusy={taskOrderBusy}
            onDelete={deleteTask}
          />
        )}

        {page === "projects" && featureEnabled("projects") && <Suspense fallback={null}><WorkPage currentUserId={user.id} currentUserLogin={user.login} profile={profile} onProfileChanged={load} onError={flashError} onChanged={() => void refreshPortfolioAndGoals()} /></Suspense>}

        {page === "tracker" && featureEnabled("tracker") && <Suspense fallback={null}><TimeTrackerPage onOpenCalories={featureEnabled("calories") ? openCalories : undefined} onOpenIntroduction={() => setTimerIntroOpen(true)} /></Suspense>}

        {page === "calories" && featureEnabled("calories") && calorieMode && (
          <TrackerPreview currentDate={currentDate} userID={user.id} calorieMode={calorieMode} standaloneCalories />
        )}

        {page === "profile" && (
          <ProfilePage
            user={user}
            themePreferences={themePreferences}
            onThemeChange={changeTheme}
            profile={profile}
            onProfileChanged={load}
            workspace={workspace}
            onWorkspaceSettings={() => setWorkspaceEditing(true)}
            trackersEnabled={trackersEnabled}
            bottomNavigation={bottomNavigation}
            bottomNavigationSaving={bottomNavigationSaving}
            onOpenCard={() => setPage("card")}
            onTrackersEnabledChange={setTrackerVisibility}
            onBottomNavigationChange={saveBottomNavigation}
            onLogout={onLogout}
            onReset={resetAll}
          />
        )}

        {page === "food-admin" && user.isAdmin && <Suspense fallback={null}><AdminFoodsPage /></Suspense>}

        {error && <div className="errorBar">{error}</div>}
      </main>

      {workspaceIntroOpen && workspace && <WorkspaceOnboarding
        initialFeatures={workspace.features}
        initialNavigation={bottomNavigation}
        editing={workspace.onboardingCompleted}
        onSave={saveWorkspace}
        onClose={() => setWorkspaceEditing(false)}
      />}
      {showTimerIntro && <TimerIntroduction onDone={finishTimerIntro} />}

      {calorieModePickerOpen && (
        <Modal title="Как хотите вести учет калорий?" onClose={() => setCalorieModePickerOpen(false)} className="calorieModeModal">
          <div className="calorieModeOptions">
            <button type="button" onClick={() => chooseCalorieMode("fatsecret")}>
              <span className="calorieModeIcon"><FlameIcon /></span>
              <span><strong>FatSecret</strong><small>Использовать внешний сервис FatSecret. Учет питания и КБЖУ будет происходить в отдельном приложении FatSecret, а Identity Workspace будет отображать синхронизированные данные.</small></span>
              <span className="calorieModeChevron" aria-hidden="true">›</span>
            </button>
            <button type="button" onClick={() => chooseCalorieMode("internal")}>
              <span className="calorieModeIcon"><FlameIcon /></span>
              <span><strong>Внутренний учет Identity Workspace</strong><small>Все операции с питанием, калориями и КБЖУ будут происходить внутри Identity Workspace.</small></span>
              <span className="calorieModeChevron" aria-hidden="true">›</span>
            </button>
          </div>
        </Modal>
      )}

      {profileEditing && (
        <ProfileEditor
          profile={profile}
          focusField={profileEditing}
          onClose={() => setProfileEditing(null)}
          onSaved={async () => {
            setProfileEditing(null);
            await load();
            addToast("Профиль обновлён", "Данные карты сохранены.", "success");
          }}
        />
      )}

      {signatureEditing && (
        <SignatureEditor
          initialValue={profile.signature}
          onClose={() => setSignatureEditing(false)}
          onSave={async (data) => {
            await api.setSignature(data);
            setSignatureEditing(false);
            await load();
          }}
        />
      )}

      {taskEditing && (
        <TaskEditor task={taskEditing} currentDate={formalTodayKey(currentDate || localTodayKey())} onClose={() => setTaskEditing(null)} onSave={saveTask} />
      )}

      {goalEditing && (
        <GoalEditor
          goal={goalEditing === "new" ? null : goalEditing}
          tasks={tasks}
          onClose={() => setGoalEditing(null)}
          onSave={saveGoal}
          onDelete={deleteGoal}
        />
      )}

      {showTimeline && (
        <PortfolioTimeline goals={portfolio.completed} onClose={() => setShowTimeline(false)} />
      )}

      <ToastStack items={toasts} onDismiss={(id) => setToasts((items) => items.filter((item) => item.id !== id))} />
    </div>
  );
}

function TrackerPreview({ currentDate, userID, calorieMode, onOpenCalories, standaloneCalories = false, widgetsEnabled = true, caloriesEnabled = true }: {
  currentDate: string;
  userID: number;
  calorieMode: CalorieAccountingMode | null;
  onOpenCalories?: () => void;
  standaloneCalories?: boolean;
  widgetsEnabled?: boolean;
  caloriesEnabled?: boolean;
}) {
  const [trackerDate, setTrackerDate] = useState(() => currentDate || localTodayKey());
  const [legacySnapshot] = useState(() => loadLegacyTrackerSnapshot(userID));
  const [selected, setSelected] = useState<TrackerSelectionID[]>(legacySnapshot.settings.selected);
  const [trackerData, setTrackerData] = useState<TrackerState | null>(null);
  const [trackerLoading, setTrackerLoading] = useState(true);
  const [trackerError, setTrackerError] = useState<string | null>(null);
  const [fatSecretStatus, setFatSecretStatus] = useState<FatSecretStatus | null>(null);
  const [nutrition, setNutrition] = useState<FatSecretNutrition | null>(null);
  const [nutritionLoading, setNutritionLoading] = useState(true);
  const [nutritionError, setNutritionError] = useState<string | null>(null);
  const [nutritionPickerMeal, setNutritionPickerMeal] = useState<FatSecretMeal | null>(null);
  const [nutritionEditingEntry, setNutritionEditingEntry] = useState<FatSecretNutritionEntry | null>(null);
  const [nutritionEntryMenu, setNutritionEntryMenu] = useState<string | null>(null);
  const [nutritionMutationBusy, setNutritionMutationBusy] = useState(false);
  const [expandedNutritionMeals, setExpandedNutritionMeals] = useState<Set<FatSecretMeal>>(() => new Set(["breakfast"]));
  const [expandedNutritionEntryLists, setExpandedNutritionEntryLists] = useState<Set<FatSecretMeal>>(() => new Set());
  const [fatSecretDisconnecting, setFatSecretDisconnecting] = useState(false);
  const [fatSecretNotice, setFatSecretNotice] = useState<FatSecretNotice | null>(consumeFatSecretCallbackNotice);
  const [nutritionGoalsOpen, setNutritionGoalsOpen] = useState(false);
  const [nutritionGoalsMode, setNutritionGoalsMode] = useState<NutritionGoalsMode>("quick");
  const [nutritionGoalsSaving, setNutritionGoalsSaving] = useState(false);
  const [nutritionGoalsError, setNutritionGoalsError] = useState<string | null>(null);
  const [draftNutritionGoals, setDraftNutritionGoals] = useState<NutritionGoals>(DEFAULT_NUTRITION_GOALS);
  const [nutritionCalculator, setNutritionCalculator] = useState<NutritionCalculatorInput>(DEFAULT_NUTRITION_CALCULATOR);
  const [calorieReportFrom, setCalorieReportFrom] = useState(() => addDaysToDateKey(trackerDate, -6));
  const [calorieReportTo, setCalorieReportTo] = useState(trackerDate);
  const [calorieReportExpanded, setCalorieReportExpanded] = useState(false);
  const [calorieReportBusy, setCalorieReportBusy] = useState(false);
  const [calorieReportError, setCalorieReportError] = useState<string | null>(null);
  const [waterSaving, setWaterSaving] = useState(false);
  const [weightSaving, setWeightSaving] = useState(false);
  const [githubData, setGitHubData] = useState<GitHubTrackerState | null>(null);
  const [githubLoading, setGitHubLoading] = useState(false);
  const [githubSaving, setGitHubSaving] = useState(false);
  const [githubError, setGitHubError] = useState<string | null>(null);
  const [draftGitHubUsername, setDraftGitHubUsername] = useState("");
  const [githubPeriod, setGitHubPeriod] = useState<GitHubPeriod>(() => loadGitHubPeriod(userID));
  const [trackerModal, setTrackerModal] = useState<TrackerModal>(null);
  const [draftSelected, setDraftSelected] = useState<TrackerSelectionID[]>(selected);
  const [draftWaterGoal, setDraftWaterGoal] = useState(legacySnapshot.settings.waterGoal);
  const [draftWaterCount, setDraftWaterCount] = useState(legacySnapshot.settings.waterByDate[trackerDate] ?? 0);
  const [draftWeight, setDraftWeight] = useState(
    legacySnapshot.hasStoredWeight ? formatTrackerWeight(legacySnapshot.settings.weightKg) : "",
  );
  const [customName, setCustomName] = useState("");
  const [customTarget, setCustomTarget] = useState("100");
  const [customStep, setCustomStep] = useState("1");
  const [customIcon, setCustomIcon] = useState("icon-01");
  const [customSaving, setCustomSaving] = useState(false);
  const [activeCustomTrackerID, setActiveCustomTrackerID] = useState<number | null>(null);
  const [customStatisticsPeriod, setCustomStatisticsPeriod] = useState<7 | 30>(7);
  const [trackerReminderDraft, setTrackerReminderDraft] = useState<Record<string, { enabled: boolean; time: string }>>({});
  const [trackerReminderLoading, setTrackerReminderLoading] = useState(false);
  const [trackerReminderSaving, setTrackerReminderSaving] = useState(false);
  const [trackerReminderError, setTrackerReminderError] = useState<string | null>(null);
  const [trackerNotificationConfig, setTrackerNotificationConfig] = useState<{ configured: boolean; publicKey: string } | null>(null);
  const [trackerNotificationState, setTrackerNotificationState] = useState<DeviceNotificationState>("unknown");
  const nutritionGoalsPromptHandled = useRef(storageFlag(nutritionGoalsPromptStorageKey(userID)));
  const trackerReady = trackerData !== null;
  const waterEntry = trackerData?.waterHistory.find((entry) => entry.date === trackerDate);
  const waterGoal = trackerData?.waterGoal ?? legacySnapshot.settings.waterGoal;
  const calorieGoal = trackerData?.calorieGoal ?? DEFAULT_NUTRITION_GOALS.calorieGoal;
  const proteinGoal = trackerData?.proteinGoal ?? DEFAULT_NUTRITION_GOALS.proteinGoal;
  const fatGoal = trackerData?.fatGoal ?? DEFAULT_NUTRITION_GOALS.fatGoal;
  const carbohydrateGoal = trackerData?.carbohydrateGoal ?? DEFAULT_NUTRITION_GOALS.carbohydrateGoal;
  const consumedCalories = nutrition?.calories ?? 0;
  const calorieProgress = Math.min(100, Math.max(0, consumedCalories / Math.max(1, calorieGoal) * 100));
  const macroProgress = {
    protein: Math.min(100, (nutrition?.protein ?? 0) / proteinGoal * 100),
    fat: Math.min(100, (nutrition?.fat ?? 0) / fatGoal * 100),
    carbohydrate: Math.min(100, (nutrition?.carbohydrate ?? 0) / carbohydrateGoal * 100),
  };
  const waterCount = trackerData ? (waterEntry?.glasses ?? 0) : (legacySnapshot.settings.waterByDate[trackerDate] ?? 0);
  const currentWeightKg = trackerData
    ? trackerData.currentWeightKg
    : (legacySnapshot.hasStoredWeight ? legacySnapshot.settings.weightKg : null);
  const githubContributionValue = githubContributionCount(githubData, githubPeriod, trackerDate);
  const githubPeriodCaption = githubPeriodMeta(githubPeriod).caption;
  const selectedCards: TrackerSelectionID[] = widgetsEnabled ? selected.filter((id) => {
    if (id === "calories" && !caloriesEnabled) return false;
    if (TRACKER_CARD_ORDER.includes(id as TrackerCardID)) return true;
    const match = /^custom:(\d+)$/.exec(id);
    return Boolean(match && trackerData?.customTrackers.some((tracker) => tracker.id === Number(match[1])));
  }) : ["calories"];
  const githubSelected = selectedCards.includes("github");
  const activeCustomTracker = trackerData?.customTrackers.find((tracker) => tracker.id === activeCustomTrackerID) ?? null;
  const parsedWeight = Number(draftWeight.replace(",", "."));
  const weightIsValid = Number.isFinite(parsedWeight) && parsedWeight >= 20 && parsedWeight <= 500;
  const calculatedNutritionGoals = calculateNutritionGoals(nutritionCalculator);
  const manualNutritionGoals: NutritionGoals = {
    ...draftNutritionGoals,
    calorieGoal: caloriesFromMacros(draftNutritionGoals.proteinGoal, draftNutritionGoals.fatGoal, draftNutritionGoals.carbohydrateGoal),
  };
  const pendingNutritionGoals = nutritionGoalsMode === "quick" ? calculatedNutritionGoals : manualNutritionGoals;
  const nutritionGoalsAreValid = pendingNutritionGoals !== null
    && Number.isInteger(pendingNutritionGoals.calorieGoal)
    && pendingNutritionGoals.calorieGoal >= 500 && pendingNutritionGoals.calorieGoal <= 10000
    && Number.isInteger(pendingNutritionGoals.proteinGoal)
    && pendingNutritionGoals.proteinGoal >= 1 && pendingNutritionGoals.proteinGoal <= 1000
    && Number.isInteger(pendingNutritionGoals.fatGoal)
    && pendingNutritionGoals.fatGoal >= 1 && pendingNutritionGoals.fatGoal <= 1000
    && Number.isInteger(pendingNutritionGoals.carbohydrateGoal)
    && pendingNutritionGoals.carbohydrateGoal >= 1 && pendingNutritionGoals.carbohydrateGoal <= 1000;
  const nutritionGoalsChanged = pendingNutritionGoals !== null && (pendingNutritionGoals.calorieGoal !== calorieGoal
    || pendingNutritionGoals.proteinGoal !== proteinGoal
    || pendingNutritionGoals.fatGoal !== fatGoal
    || pendingNutritionGoals.carbohydrateGoal !== carbohydrateGoal);
  const lastFatSecretAutoRefresh = useRef(0);

  useEffect(() => {
    if (currentDate) setTrackerDate(currentDate);
  }, [currentDate]);

  useEffect(() => {
    let midnightTimer: number | undefined;
    const updateDate = () => setTrackerDate(localTodayKey());
    const scheduleMidnight = () => {
      const now = new Date();
      const nextMidnight = new Date(now);
      nextMidnight.setHours(24, 0, 1, 0);
      midnightTimer = window.setTimeout(() => {
        updateDate();
        scheduleMidnight();
      }, Math.max(1_000, nextMidnight.getTime() - now.getTime()));
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") updateDate();
    };
    scheduleMidnight();
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("pageshow", updateDate);
    return () => {
      if (midnightTimer !== undefined) window.clearTimeout(midnightTimer);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("pageshow", updateDate);
    };
  }, []);

  useEffect(() => {
    persistGitHubPeriod(userID, githubPeriod);
  }, [githubPeriod, userID]);

  useEffect(() => {
    let active = true;

    async function loadTrackers() {
      setTrackerLoading(true);
      setTrackerError(null);
      let serverState: TrackerState | null = null;
      try {
        serverState = await api.trackers();
        let alreadyImported = false;
        try {
          alreadyImported = window.localStorage.getItem(trackerServerImportKey(userID)) === "1";
        } catch {
          // Серверные проверки ниже делают повторный импорт идемпотентным.
        }

        if (!alreadyImported) {
          const serverWaterDates = new Set(serverState.waterHistory.map((entry) => entry.date));
          const importGoal = serverState.waterHistory.length > 0
            ? serverState.waterGoal
            : legacySnapshot.settings.waterGoal;
          const legacyWater = Object.entries(legacySnapshot.settings.waterByDate)
            .filter(([date]) => parseDateKey(date) !== null)
            .sort(([left], [right]) => left.localeCompare(right));

          for (const [date, glasses] of legacyWater) {
            if (serverWaterDates.has(date)) continue;
            const entry = await api.saveTrackerWater(date, glasses, importGoal);
            serverState = trackerStateWithWater(serverState, entry);
            serverWaterDates.add(date);
          }

          if (serverState.weightHistory.length === 0 && legacySnapshot.hasStoredWeight) {
            const entry = await api.saveTrackerWeight(trackerDate, legacySnapshot.settings.weightKg);
            serverState = trackerStateWithWeight(serverState, entry);
          }

          try {
            window.localStorage.setItem(trackerServerImportKey(userID), "1");
          } catch {
            // При недоступном localStorage даты на сервере защитят данные от перезаписи.
          }
        }

        if (active) setTrackerData(serverState);
      } catch {
        if (active) {
          if (serverState) setTrackerData(serverState);
          setTrackerError("Не удалось синхронизировать трекеры");
        }
      } finally {
        if (active) setTrackerLoading(false);
      }
    }

    void loadTrackers();
    return () => {
      active = false;
    };
  }, [legacySnapshot, trackerDate]);

  useEffect(() => {
    if (!githubSelected) return;
    let active = true;
    setGitHubLoading(true);
    setGitHubError(null);
    api.githubTracker()
      .then((state) => {
        if (!active) return;
        setGitHubData(state);
        setDraftGitHubUsername(state.username);
      })
      .catch((cause: unknown) => {
        if (active) setGitHubError(cause instanceof Error ? cause.message : "Не удалось загрузить GitHub");
      })
      .finally(() => {
        if (active) setGitHubLoading(false);
      });
    return () => {
      active = false;
    };
  }, [githubSelected]);

  useEffect(() => {
    let active = true;

    async function loadFatSecret() {
      setNutritionLoading(true);
      setNutritionError(null);
      setNutrition(null);
      try {
        const status = await api.fatSecretStatus();
        if (!active) return;
        setFatSecretStatus(status);
        const dailyNutrition = calorieMode === "fatsecret"
          ? await api.fatSecretNutrition(trackerDate)
          : await api.localNutrition(trackerDate);
        if (active) {
          setNutrition(dailyNutrition);
          lastFatSecretAutoRefresh.current = Date.now();
        }
      } catch {
        if (active) {
          setNutrition(null);
          setNutritionError("Не удалось получить данные FatSecret");
        }
      } finally {
        if (active) setNutritionLoading(false);
      }
    }

    void loadFatSecret();
    return () => {
      active = false;
    };
  }, [calorieMode, trackerDate]);

  useEffect(() => {
    if (!fatSecretStatus) return;

    const refreshWhenVisible = () => {
      if (document.visibilityState !== "visible") return;
      if (Date.now() - lastFatSecretAutoRefresh.current < 5_000) return;
      lastFatSecretAutoRefresh.current = Date.now();
      void refreshFatSecretNutrition(true);
    };

    window.addEventListener("pageshow", refreshWhenVisible);
    window.addEventListener("focus", refreshWhenVisible);
    document.addEventListener("visibilitychange", refreshWhenVisible);
    return () => {
      window.removeEventListener("pageshow", refreshWhenVisible);
      window.removeEventListener("focus", refreshWhenVisible);
      document.removeEventListener("visibilitychange", refreshWhenVisible);
    };
  }, [fatSecretStatus?.connected, trackerDate, nutritionLoading]);

  useEffect(() => {
    if (fatSecretNotice) setTrackerModal("calories");
  }, [fatSecretNotice]);

  useEffect(() => {
    const caloriesOpen = standaloneCalories || trackerModal === "calories";
    if (!caloriesOpen || !trackerReady || nutritionGoalsPromptHandled.current) return;
    nutritionGoalsPromptHandled.current = true;
    persistStorageFlag(nutritionGoalsPromptStorageKey(userID));
    openNutritionGoals();
  }, [standaloneCalories, trackerModal, trackerReady, userID]);

  useEffect(() => {
    setCalorieReportFrom(addDaysToDateKey(trackerDate, -6));
    setCalorieReportTo(trackerDate);
    setCalorieReportError(null);
  }, [trackerDate]);

  useEffect(() => {
    if (!trackerModal) return;
    const previousOverflow = document.body.style.overflow;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setTrackerModal(null);
    };
    document.body.style.overflow = "hidden";
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [trackerModal]);

  async function openTrackerReminders() {
    setTrackerModal("reminders");
    setTrackerReminderLoading(true);
    setTrackerReminderError(null);
    try {
      const [loaded, config] = await Promise.all([api.trackerReminders(), api.notificationConfig()]);
      const byKey = new Map(loaded.map((item) => [item.trackerKey, item]));
      const keys: TrackerSelectionID[] = [
        ...TRACKER_REMINDER_CARD_ORDER,
        ...(trackerData?.customTrackers ?? []).map((tracker) => `custom:${tracker.id}` as TrackerSelectionID),
      ];
      const draft: Record<string, { enabled: boolean; time: string }> = {};
      keys.forEach((key) => {
        const reminder = byKey.get(key);
        draft[key] = { enabled: reminder?.enabled ?? false, time: reminder?.time || "20:00" };
      });
      setTrackerReminderDraft(draft);
      setTrackerNotificationConfig(config);
      if (!("Notification" in window) || !("serviceWorker" in navigator) || !("PushManager" in window) || !config.configured) {
        setTrackerNotificationState("unsupported");
      } else if (Notification.permission === "denied") {
        setTrackerNotificationState("denied");
      } else if (Notification.permission === "granted") {
        setTrackerNotificationState(await registerDeviceNotifications(false, config));
      } else {
        setTrackerNotificationState("unknown");
      }
    } catch (cause) {
      setTrackerReminderError(cause instanceof Error ? cause.message : "Не удалось загрузить напоминания");
    } finally {
      setTrackerReminderLoading(false);
    }
  }

  async function saveTrackerReminders() {
    if (trackerReminderSaving) return;
    setTrackerReminderSaving(true);
    setTrackerReminderError(null);
    try {
      const entries = Object.entries(trackerReminderDraft);
      const saved = await Promise.all(entries.map(([trackerKey, reminder]) =>
        api.saveTrackerReminder({ trackerKey, time: reminder.time || "20:00", enabled: reminder.enabled }),
      ));
      const nextDraft: Record<string, { enabled: boolean; time: string }> = {};
      saved.forEach((item: TrackerReminder) => { nextDraft[item.trackerKey] = { enabled: item.enabled, time: item.time }; });
      setTrackerReminderDraft(nextDraft);
      setTrackerModal(null);
    } catch (cause) {
      setTrackerReminderError(cause instanceof Error ? cause.message : "Не удалось сохранить напоминания");
    } finally {
      setTrackerReminderSaving(false);
    }
  }

  async function enableTrackerNotifications() {
    const state = await registerDeviceNotifications(true, trackerNotificationConfig ?? undefined);
    setTrackerNotificationState(state);
  }

  function openPicker() {
    setDraftSelected([...selected]);
    setTrackerModal("picker");
  }

  function toggleDraftCard(card: TrackerSelectionID) {
    setDraftSelected((items) => {
      if (items.includes(card)) return items.filter((item) => item !== card);
      if (items.length >= MAX_TRACKER_CARDS) return items;
      return [...items, card];
    });
  }

  function saveTrackerSelection() {
    const nextSelected = [...draftSelected];
    setSelected(nextSelected);
    persistTrackerSelection(userID, nextSelected);
    setTrackerModal(null);
  }

  function openCaloriesTracker() {
    if (onOpenCalories) {
      onOpenCalories();
      return;
    }
    setNutritionError(null);
    setNutritionGoalsError(null);
    setTrackerModal("calories");
  }

  function openNutritionPicker(meal: FatSecretMeal, entry: FatSecretNutritionEntry | null = null) {
    setNutritionError(null);
    setNutritionPickerMeal(meal);
    setNutritionEditingEntry(entry);
    setNutritionEntryMenu(null);
  }

  function openNutritionGoals() {
    setDraftNutritionGoals({ calorieGoal, proteinGoal, fatGoal, carbohydrateGoal });
    setNutritionCalculator((current) => ({
      ...current,
      weightKg: currentWeightKg !== null ? String(currentWeightKg) : current.weightKg,
    }));
    setNutritionGoalsMode("quick");
    setNutritionGoalsError(null);
    setNutritionGoalsOpen(true);
  }

  function closeNutritionPicker() {
    setNutritionPickerMeal(null);
    setNutritionEditingEntry(null);
  }

  function acceptNutritionMutation(next: FatSecretNutrition) {
    closeNutritionPicker();
    if (!next.fetchedAt) {
      void refreshFatSecretNutrition(true);
      return;
    }
    setNutrition(next);
    setNutritionError(null);
    lastFatSecretAutoRefresh.current = Date.now();
  }

  function toggleNutritionMeal(meal: FatSecretMeal) {
    setExpandedNutritionMeals((current) => {
      const next = new Set(current);
      if (next.has(meal)) next.delete(meal);
      else next.add(meal);
      return next;
    });
  }

  function toggleNutritionEntryList(meal: FatSecretMeal) {
    setExpandedNutritionEntryLists((current) => {
      const next = new Set(current);
      if (next.has(meal)) next.delete(meal);
      else next.add(meal);
      return next;
    });
  }

  async function deleteNutritionEntry(entry: FatSecretNutritionEntry) {
    if (nutritionMutationBusy || !window.confirm(`Удалить «${entry.name || entry.description}» из дневника?`)) return;
    setNutritionMutationBusy(true);
    setNutritionError(null);
    try {
      acceptNutritionMutation(await api.deleteLocalNutritionEntry(entry.id, trackerDate));
    } catch (cause) {
      setNutritionError(cause instanceof Error ? cause.message : "Не удалось удалить продукт");
    } finally {
      setNutritionMutationBusy(false);
      setNutritionEntryMenu(null);
    }
  }

  async function refreshFatSecretNutrition(silent = false) {
    if (nutritionLoading) return;
    if (!silent) setNutritionLoading(true);
    setNutritionError(null);
    try {
      // Entries created in this app live in the local diary. Reloading the
      // FatSecret diary here replaced those entries with a different data set.
      setNutrition(calorieMode === "fatsecret"
        ? await api.fatSecretNutrition(trackerDate)
        : await api.localNutrition(trackerDate));
      lastFatSecretAutoRefresh.current = Date.now();
    } catch {
      setNutritionError("Не удалось загрузить дневник питания");
    } finally {
      if (!silent) setNutritionLoading(false);
    }
  }

  async function saveNutritionGoals() {
    if (!trackerData || nutritionGoalsSaving || !nutritionGoalsAreValid || pendingNutritionGoals === null) return;
    setNutritionGoalsSaving(true);
    setNutritionGoalsError(null);
    try {
      const saved = await api.saveNutritionGoals(pendingNutritionGoals);
      setTrackerData((current) => current ? { ...current, ...saved } : current);
      setDraftNutritionGoals(saved);
      setNutritionGoalsOpen(false);
    } catch {
      setNutritionGoalsError("Не удалось сохранить дневные нормы");
    } finally {
      setNutritionGoalsSaving(false);
    }
  }

  async function createCalorieReport() {
    if (calorieReportBusy) return;
    const reportDates = dateKeysInRange(calorieReportFrom, calorieReportTo, 31);
    if (!reportDates) {
      setCalorieReportError("Выберите корректный период продолжительностью не более 31 дня.");
      return;
    }
    setCalorieReportBusy(true);
    setCalorieReportError(null);
    try {
      const byDate = new Map<string, FatSecretNutrition>();
      if (nutrition && nutrition.date === trackerDate && reportDates.includes(trackerDate)) byDate.set(trackerDate, nutrition);
      const missing = reportDates.filter((date) => !byDate.has(date));
      let cursor = 0;
      async function worker() {
        while (cursor < missing.length) {
          const date = missing[cursor++];
          byDate.set(date, calorieMode === "fatsecret" ? await api.fatSecretNutrition(date) : await api.localNutrition(date));
        }
      }
      await Promise.all(Array.from({ length: Math.min(3, missing.length) }, () => worker()));
      await downloadCalorieReport(reportDates.map((date) => byDate.get(date)).filter((day): day is FatSecretNutrition => Boolean(day)), calorieReportFrom, calorieReportTo);
    } catch {
      setCalorieReportError("Не удалось собрать PDF из дневника питания. Попробуйте ещё раз.");
    } finally {
      setCalorieReportBusy(false);
    }
  }

  async function disconnectFatSecret() {
    if (fatSecretDisconnecting) return;
    setFatSecretDisconnecting(true);
    setNutritionError(null);
    try {
      await api.disconnectFatSecret();
      setFatSecretStatus((current) => current ? { ...current, connected: false, connectedAt: "" } : { configured: true, connected: false, connectedAt: "" });
      setNutrition(null);
      setFatSecretNotice(null);
      setTrackerModal(null);
    } catch {
      setNutritionError("Не удалось отключить FatSecret");
    } finally {
      setFatSecretDisconnecting(false);
    }
  }

  function openWaterTracker() {
    setDraftWaterGoal(waterGoal);
    setDraftWaterCount(waterCount);
    setTrackerError(null);
    setTrackerModal("water");
  }

  async function saveWaterTracker() {
    if (!trackerData || waterSaving) return;
    setWaterSaving(true);
    setTrackerError(null);
    try {
      const entry = await api.saveTrackerWater(trackerDate, draftWaterCount, draftWaterGoal);
      setTrackerData((current) => current ? trackerStateWithWater(current, entry) : current);
      setTrackerModal(null);
    } catch {
      setTrackerError("Не удалось сохранить воду");
    } finally {
      setWaterSaving(false);
    }
  }

  async function addWaterGlass() {
    if (!trackerData || waterSaving || waterCount >= 99) return;
    const previousState = trackerData;
    const nextCount = waterCount + 1;
    const optimisticEntry: TrackerWaterEntry = {
      date: trackerDate,
      glasses: nextCount,
      goalGlasses: waterGoal,
      updatedAt: new Date().toISOString(),
    };
    setWaterSaving(true);
    setTrackerError(null);
    setTrackerData(trackerStateWithWater(previousState, optimisticEntry));
    try {
      const entry = await api.saveTrackerWater(trackerDate, nextCount, waterGoal);
      setTrackerData((current) => current ? trackerStateWithWater(current, entry) : current);
    } catch {
      setTrackerData(previousState);
      setTrackerError("Не удалось добавить стакан");
    } finally {
      setWaterSaving(false);
    }
  }

  function openWeightTracker() {
    setDraftWeight(currentWeightKg === null ? "" : formatTrackerWeight(currentWeightKg));
    setTrackerError(null);
    setTrackerModal("weight");
  }

  function openGitHubTracker() {
    setDraftGitHubUsername(githubData?.username ?? "");
    setGitHubError(null);
    setTrackerModal("github");
  }

  async function saveGitHubTracker(event: React.FormEvent) {
    event.preventDefault();
    const username = draftGitHubUsername.trim();
    if (!username || githubSaving) return;
    setGitHubSaving(true);
    setGitHubError(null);
    try {
      const state = await api.saveGitHubTracker(username);
      setGitHubData(state);
      setDraftGitHubUsername(state.username);
      setTrackerModal(null);
    } catch (cause) {
      setGitHubError(cause instanceof Error ? cause.message : "Не удалось подключить GitHub");
    } finally {
      setGitHubSaving(false);
    }
  }

  async function refreshGitHubTracker() {
    if (githubLoading || githubSaving || !githubData?.configured) return;
    setGitHubLoading(true);
    setGitHubError(null);
    try {
      setGitHubData(await api.githubTracker(true));
    } catch (cause) {
      setGitHubError(cause instanceof Error ? cause.message : "Не удалось обновить GitHub");
    } finally {
      setGitHubLoading(false);
    }
  }

  async function deleteGitHubTracker() {
    if (!githubData?.configured || githubSaving) return;
    if (!window.confirm("Отключить GitHub-трекер?")) return;
    setGitHubSaving(true);
    setGitHubError(null);
    try {
      await api.deleteGitHubTracker();
      setGitHubData({ configured: false, username: "", todayContributions: 0, weekContributions: 0, yearContributions: 0, days: [], fetchedAt: "", stale: false, source: "" });
      setDraftGitHubUsername("");
      setTrackerModal(null);
    } catch (cause) {
      setGitHubError(cause instanceof Error ? cause.message : "Не удалось отключить GitHub");
    } finally {
      setGitHubSaving(false);
    }
  }

  async function saveWeightTracker() {
    if (!trackerData || !weightIsValid || weightSaving) return;
    setWeightSaving(true);
    setTrackerError(null);
    try {
      const weightKg = trackerDecimal(parsedWeight, 20, 500, parsedWeight);
      const entry = await api.saveTrackerWeight(trackerDate, weightKg);
      setTrackerData((current) => current ? trackerStateWithWeight(current, entry) : current);
      setTrackerModal(null);
    } catch {
      setTrackerError("Не удалось сохранить вес");
    } finally {
      setWeightSaving(false);
    }
  }

  function openCustomTrackerEditor(tracker?: CustomTracker) {
    setCustomName(tracker?.name ?? "");
    setCustomTarget(String(tracker?.targetValue ?? 100));
    setCustomStep(String(tracker?.stepValue ?? 1));
    setCustomIcon(tracker?.icon ?? "icon-01");
    setTrackerError(null);
    setTrackerModal(tracker ? "custom-edit" : "custom-create");
  }

  async function saveCustomTracker(event: React.FormEvent) {
    event.preventDefault();
    if (customSaving) return;
    const input: CustomTrackerInput = {
      name: customName.trim(),
      targetValue: Number(customTarget.replace(",", ".")),
      stepValue: Number(customStep.replace(",", ".")),
      icon: customIcon,
    };
    if (!input.name || !Number.isFinite(input.targetValue) || !Number.isFinite(input.stepValue) || input.targetValue <= 0 || input.stepValue <= 0 || input.stepValue > input.targetValue) return;
    setCustomSaving(true);
    setTrackerError(null);
    try {
      if (trackerModal === "custom-edit") {
        if (!activeCustomTracker) return;
        const updated = await api.updateCustomTracker(activeCustomTracker.id, input);
        setTrackerData((current) => current ? {
          ...current,
          customTrackers: current.customTrackers.map((tracker) => tracker.id === updated.id ? updated : tracker),
        } : current);
        setTrackerModal("custom");
        return;
      }
      const created = await api.createCustomTracker(input);
      setTrackerData((current) => current ? { ...current, customTrackers: [...current.customTrackers, created] } : current);
      const selectionID = `custom:${created.id}` as TrackerSelectionID;
      setSelected((items) => {
        const next = items.length < MAX_TRACKER_CARDS ? [...items, selectionID] : items;
        persistTrackerSelection(userID, next);
        return next;
      });
      setDraftSelected((items) => items.length < MAX_TRACKER_CARDS ? [...items, selectionID] : items);
      setCustomName(""); setCustomTarget("100"); setCustomStep("1"); setCustomIcon("icon-01");
      setTrackerModal("picker");
    } catch (cause) {
      setTrackerError(cause instanceof Error ? cause.message : "Не удалось сохранить трекер");
    } finally { setCustomSaving(false); }
  }

  async function stepCustomTracker(tracker: CustomTracker, direction: -1 | 1) {
    if (customSaving) return;
    setCustomSaving(true);
    setTrackerError(null);
    try {
      const updated = await api.stepCustomTracker(tracker.id, trackerDate, direction);
      setTrackerData((current) => current ? trackerStateWithCustom(current, updated, trackerDate) : current);
    } catch { setTrackerError("Не удалось обновить трекер"); }
    finally { setCustomSaving(false); }
  }

  async function deleteCustomTracker(tracker: CustomTracker) {
    if (!window.confirm(`Удалить трекер «${tracker.name}»?`)) return;
    setCustomSaving(true);
    try {
      await api.deleteCustomTracker(tracker.id);
      const selectionID = `custom:${tracker.id}` as TrackerSelectionID;
      setTrackerData((current) => current ? { ...current, customTrackers: current.customTrackers.filter((item) => item.id !== tracker.id) } : current);
      setSelected((items) => { const next = items.filter((item) => item !== selectionID); persistTrackerSelection(userID, next); return next; });
      setDraftSelected((items) => items.filter((item) => item !== selectionID));
      setTrackerModal(null);
    } catch { setTrackerError("Не удалось удалить трекер"); }
    finally { setCustomSaving(false); }
  }

  return (
    <>
      <section className={`trackerPreview${standaloneCalories ? " trackerPreviewStandaloneHidden" : ""}`} aria-label="Трекеры">
        <header className="trackerToolbar">
          <div>
            <strong>Трекеры</strong>
            <span
              aria-live="polite"
              role={trackerError ? "alert" : "status"}
              style={trackerError ? { color: "var(--stamp)" } : undefined}
            >
              {trackerError ?? (trackerLoading ? "Синхронизация…" : `${selectedCards.length} ${activeTrackerLabel(selectedCards.length)}`)}
            </span>
          </div>
          <div className="trackerToolbarActions">
            <button type="button" className="trackerSettingsButton trackerStatisticsButton" onClick={() => setTrackerModal("statistics")} disabled={!trackerReady} aria-label="Открыть статистику трекеров">
              <TrackerStatisticsIcon /><span>Статистика</span>
            </button>
            {widgetsEnabled && <button type="button" className="trackerSettingsButton" onClick={openPicker} aria-label="Настроить трекеры">
              <TrackerSlidersIcon /><span>Настроить</span>
            </button>}
          </div>
        </header>

        {selectedCards.length === 0 && (
          <button type="button" className="trackerEmpty" onClick={openPicker}>
            <span aria-hidden="true">＋</span>
            <strong>Выберите трекер</strong>
            <small>Здоровье, GitHub или свой показатель</small>
          </button>
        )}

        {selectedCards.map((card) => {
          if (card === "calories") {
            const caloriesValue = nutrition ? Math.round(nutrition.calories) : null;
            const usesFatSecret = calorieMode === "fatsecret";
            const statusPending = usesFatSecret && fatSecretStatus === null;
            const caloriesLabel = !calorieMode
              ? "выбрать способ учета"
              : !usesFatSecret
                ? nutritionLoading && caloriesValue === null ? "загрузка…" : `из ${formatNutrition(calorieGoal, 0)} ккал`
              : statusPending
              ? "проверка подключения…"
              : !fatSecretStatus?.configured
                ? "нужны API-ключи"
                : !fatSecretStatus?.connected
                  ? "подключить аккаунт"
                  : nutritionLoading && caloriesValue === null
                    ? "синхронизация…"
                    : nutritionError && caloriesValue === null
                      ? "нет данных"
                      : `из ${formatNutrition(calorieGoal, 0)} ккал`;
            const calorieProgress = caloriesValue === null
              ? 0
              : Math.min(100, Math.max(0, caloriesValue / calorieGoal * 100));
            const fatSecretDisconnected = usesFatSecret && !statusPending && !fatSecretStatus?.connected;
            return (
              <article className={`trackerMetric trackerMetricCalories trackerMetricInteractive${fatSecretDisconnected ? " trackerMetricCaloriesDisconnected" : ""}`} key={card}>
                <header className="trackerMetricHead"><FlameIcon /><span>Калории</span></header>
                {fatSecretDisconnected ? (
                  <div className="fatSecretTrackerPrompt">
                    <strong>Подключи FatSecret</strong>
                  </div>
                ) : (
                  <>
                    <div className="trackerReading">
                      <strong>{caloriesValue === null ? (statusPending ? "…" : "—") : formatNutrition(caloriesValue, 0)}</strong>
                      <span>{caloriesLabel}</span>
                    </div>
                    <RingChart progress={calorieProgress} />
                  </>
                )}
                <button type="button" className="trackerMetricAction" onClick={openCaloriesTracker} aria-label={!calorieMode ? "Выбрать способ учета калорий" : fatSecretDisconnected ? "Подключить FatSecret" : "Открыть калории и КБЖУ"} />
              </article>
            );
          }
          if (card === "water") {
            return (
              <article className="trackerMetric trackerMetricWater trackerMetricInteractive" key={card}>
                <header className="trackerMetricHead"><WaterIcon /><span>Вода</span></header>
                <div className="trackerReading"><strong>{waterCount}</strong><span>из {waterGoal} стаканов</span></div>
                <RingChart progress={Math.min(100, waterCount / waterGoal * 100)} />
                <button type="button" className="trackerWaterQuickAdd" onClick={() => void addWaterGlass()} disabled={!trackerReady || waterSaving || waterCount >= 99} aria-busy={waterSaving} aria-label="Добавить стакан воды"><span aria-hidden="true">＋</span></button>
                <button type="button" className="trackerMetricAction" onClick={openWaterTracker} disabled={!trackerReady} aria-label={`Вода: выпито ${waterCount} из ${waterGoal} стаканов. Открыть настройки`} />
              </article>
            );
          }
          if (card === "github") {
            const configured = Boolean(githubData?.configured);
            const githubLabel = githubLoading && !githubData
              ? "обновление…"
              : githubError && !githubData
                ? "не удалось загрузить"
                : !configured
                  ? "укажите username"
                  : githubData?.stale
                    ? `${githubPeriodCaption} · кэш`
                    : githubPeriodCaption;
            return (
              <article className="trackerMetric trackerMetricGitHub trackerMetricInteractive" key={card}>
                <header className="trackerMetricHead"><GitHubIcon /><span>GitHub</span></header>
                <div className="trackerReading githubTrackerReading"><strong>{configured ? githubContributionValue : "—"}</strong><span>{githubLabel}</span></div>
                <GitHubHeatmap days={githubData?.days ?? []} weeks={GITHUB_TRACKER_CARD_WEEKS} />
                <button type="button" className="trackerMetricAction" onClick={openGitHubTracker} aria-label={configured ? `GitHub ${githubData?.username}: ${githubContributionValue} ${githubPeriodCaption}. Открыть настройки` : "Настроить GitHub-трекер"} />
              </article>
            );
          }
          const customID = /^custom:(\d+)$/.exec(card)?.[1];
          if (customID) {
            const tracker = trackerData?.customTrackers.find((item) => item.id === Number(customID));
            if (!tracker) return null;
            const progress = Math.min(100, tracker.currentValue / tracker.targetValue * 100);
            return (
              <article className="trackerMetric trackerMetricCustom trackerMetricInteractive" key={card}>
                <header className="trackerMetricHead"><span className="customTrackerGlyph" aria-hidden="true"><CustomTrackerIcon icon={tracker.icon} /></span><span>{tracker.name}</span></header>
                <div className="trackerReading"><strong>{formatTrackerNumber(tracker.currentValue)}</strong><span>из {formatTrackerNumber(tracker.targetValue)}</span></div>
                <RingChart progress={progress} />
                <button
                  type="button"
                  className="trackerWaterQuickAdd customTrackerQuickAdd"
                  onClick={() => void stepCustomTracker(tracker, 1)}
                  disabled={customSaving || tracker.currentValue >= tracker.targetValue}
                  aria-label={`Добавить ${formatTrackerNumber(tracker.stepValue)} к показателю ${tracker.name}`}
                ><span aria-hidden="true">＋</span></button>
                <button type="button" className="trackerMetricAction" onClick={() => { setActiveCustomTrackerID(tracker.id); setCustomStatisticsPeriod(7); setTrackerError(null); setTrackerModal("custom"); }} aria-label={`${tracker.name}: ${formatTrackerNumber(tracker.currentValue)} из ${formatTrackerNumber(tracker.targetValue)}. Открыть`} />
              </article>
            );
          }
          return (
            <article className="trackerMetric trackerMetricWeight trackerMetricInteractive" key={card}>
              <header className="trackerMetricHead"><DumbbellIcon /><span>Вес</span></header>
              <div className="trackerReading"><strong>{currentWeightKg === null ? "—" : formatTrackerWeight(currentWeightKg)}</strong><span>кг</span></div>
              <WeightLine />
              <button type="button" className="trackerMetricAction" onClick={openWeightTracker} disabled={!trackerReady} aria-label={currentWeightKg === null ? "Добавить вес" : `Вес: ${formatTrackerWeight(currentWeightKg)} килограмма. Изменить`} />
            </article>
          );
        })}
      </section>

      {trackerModal === "statistics" && trackerData && (
        <Modal title="Статистика" wide className="trackerStatisticsModal" onClose={() => setTrackerModal(null)}>
          <TrackerStatistics
            currentDate={trackerDate}
            selected={selectedCards}
            state={trackerData}
            nutrition={nutrition}
            fatSecretConnected={Boolean(fatSecretStatus?.connected)}
            calorieMode={calorieMode}
            calorieGoal={calorieGoal}
            github={githubData}
            githubPeriod={githubPeriod}
            onGitHubPeriod={setGitHubPeriod}
          />
          <div className="modalActions trackerStatisticsActions"><button type="button" className="primaryButton" onClick={() => setTrackerModal(null)}>Готово</button></div>
        </Modal>
      )}

      {trackerModal === "picker" && (
        <Modal title="Трекеры" onClose={() => setTrackerModal(null)}>
          <div className="trackerPickerIntro">
            <strong>Выберите до {MAX_TRACKER_CARDS} карточек</strong>
            <span>Они появятся под вашей картой.</span>
            <span>Созданные вами трекеры можно удалить в их настройках.</span>
          </div>
          <div className="trackerPickerList">
            {TRACKER_CARD_ORDER.filter((card) => card === "calories" ? caloriesEnabled : widgetsEnabled).map((card) => {
              const selected = draftSelected.includes(card);
              const disabled = !selected && draftSelected.length >= MAX_TRACKER_CARDS;
              return (
                <button
                  type="button"
                  className={`trackerChoice ${selected ? "trackerChoiceSelected" : ""}`}
                  key={card}
                  aria-pressed={selected}
                  disabled={disabled}
                  onClick={() => toggleDraftCard(card)}
                >
                  <span className="trackerChoiceIcon"><TrackerCardIcon card={card} /></span>
                  <span className="trackerChoiceCopy"><strong>{TRACKER_CARD_META[card].title}</strong><small>{TRACKER_CARD_META[card].description}</small></span>
                  <span className="trackerChoiceCheck" aria-hidden="true">{selected ? "✓" : ""}</span>
                </button>
              );
            })}
            {widgetsEnabled && (trackerData?.customTrackers ?? []).map((tracker) => {
              const card = `custom:${tracker.id}` as TrackerSelectionID;
              const selected = draftSelected.includes(card);
              const disabled = !selected && draftSelected.length >= MAX_TRACKER_CARDS;
              return (
                <button
                  type="button"
                  className={`trackerChoice trackerChoiceCustom ${selected ? "trackerChoiceSelected" : ""}`}
                  key={card}
                  aria-pressed={selected}
                  disabled={disabled}
                  onClick={() => toggleDraftCard(card)}
                >
                  <span className="trackerChoiceIcon customTrackerGlyph" aria-hidden="true"><CustomTrackerIcon icon={tracker.icon} /></span>
                  <span className="trackerChoiceCopy"><strong>{tracker.name}</strong><small>{formatTrackerNumber(tracker.currentValue)} из {formatTrackerNumber(tracker.targetValue)}, шаг {formatTrackerNumber(tracker.stepValue)}</small></span>
                  <span className="trackerChoiceCheck" aria-hidden="true">{selected ? "✓" : ""}</span>
                </button>
              );
            })}
            {widgetsEnabled && <button type="button" className="trackerChoice trackerChoiceCreate" onClick={() => openCustomTrackerEditor()}>
              <span className="trackerChoiceIcon" aria-hidden="true">＋</span>
              <span className="trackerChoiceCopy"><strong>Создать свой трекер</strong><small>Название, цель, шаг и иконка</small></span>
              <span className="trackerChoiceCheck" aria-hidden="true">›</span>
            </button>}
          </div>
          <div className="trackerPickerCount">Выбрано: <strong>{draftSelected.length} из {MAX_TRACKER_CARDS}</strong></div>
          <button type="button" className="trackerReminderLaunch" onClick={() => void openTrackerReminders()}>
            <span className="trackerReminderBell" aria-hidden="true">◷</span>
            <span><strong>Напоминания</strong><small>Ежедневные уведомления для трекеров</small></span>
            <b aria-hidden="true">›</b>
          </button>
          <div className="modalActions">
            <button type="button" className="secondaryButton" onClick={() => setTrackerModal(null)}>Отмена</button>
            <button type="button" className="primaryButton" onClick={saveTrackerSelection}>Готово</button>
          </div>
        </Modal>
      )}


      {trackerModal === "reminders" && (
        <Modal title="Напоминания трекеров" onClose={() => setTrackerModal("picker")}>
          <div className="trackerReminderIntro">
            <strong>Ежедневные напоминания</strong>
            <span>Выберите время отдельно для каждого трекера. Уведомление придёт через Web Push, даже когда приложение закрыто.</span>
          </div>
          {trackerReminderLoading ? (
            <div className="trackerReminderLoading">Загрузка…</div>
          ) : (
            <div className="trackerReminderList">
              {[...TRACKER_REMINDER_CARD_ORDER, ...(trackerData?.customTrackers ?? []).map((tracker) => `custom:${tracker.id}` as TrackerSelectionID)].map((card) => {
                const customID = /^custom:(\d+)$/.exec(card)?.[1];
                const custom = customID ? trackerData?.customTrackers.find((item) => item.id === Number(customID)) : null;
                const title = custom?.name ?? TRACKER_CARD_META[card as TrackerCardID]?.title ?? "Трекер";
                const reminder = trackerReminderDraft[card] ?? { enabled: false, time: "20:00" };
                return (
                  <div className={`trackerReminderRow ${reminder.enabled ? "isEnabled" : ""}`} key={card}>
                    <span className="trackerReminderIcon" aria-hidden="true">{custom ? <CustomTrackerIcon icon={custom.icon} /> : <TrackerCardIcon card={card as TrackerCardID} />}</span>
                    <div className="trackerReminderCopy"><strong>{title}</strong><small>{reminder.enabled ? `Каждый день в ${reminder.time}` : "Выключено"}</small></div>
                    <input
                      className="trackerReminderTime"
                      type="time"
                      value={reminder.time}
                      disabled={!reminder.enabled}
                      aria-label={`Время напоминания: ${title}`}
                      onChange={(event) => setTrackerReminderDraft((items) => ({ ...items, [card]: { ...reminder, time: event.target.value } }))}
                    />
                    <label className="trackerReminderSwitch">
                      <input
                        type="checkbox"
                        checked={reminder.enabled}
                        aria-label={`Напоминание: ${title}`}
                        onChange={(event) => setTrackerReminderDraft((items) => ({ ...items, [card]: { ...reminder, enabled: event.target.checked } }))}
                      />
                      <span aria-hidden="true" />
                    </label>
                  </div>
                );
              })}
            </div>
          )}
          {Object.values(trackerReminderDraft).some((item) => item.enabled) && trackerNotificationState !== "enabled" && (
            <div className="notificationSetup trackerNotificationSetup">
              <div><strong>Уведомления на устройство</strong><span>{trackerNotificationState === "denied" ? "Разрешение заблокировано в настройках браузера." : trackerNotificationState === "unsupported" ? "Web Push недоступен в этом браузере или режиме. На iPhone установите PWA на экран «Домой»." : "Разрешите уведомления, чтобы напоминания приходили при закрытом приложении."}</span></div>
              {trackerNotificationState !== "denied" && trackerNotificationState !== "unsupported" && <button type="button" className="secondaryButton" onClick={() => void enableTrackerNotifications()}>Включить</button>}
            </div>
          )}
          {trackerReminderError && <div className="formError" role="alert">{trackerReminderError}</div>}
          <div className="modalActions"><button type="button" className="secondaryButton" onClick={() => setTrackerModal("picker")}>Назад</button><button type="button" className="primaryButton" disabled={trackerReminderLoading || trackerReminderSaving} onClick={() => void saveTrackerReminders()}>{trackerReminderSaving ? "Сохранение…" : "Сохранить"}</button></div>
        </Modal>
      )}

      {trackerModal === "github" && (
        <Modal title="GitHub активность" onClose={() => setTrackerModal(null)}>
          <form className="githubTrackerForm" noValidate onSubmit={saveGitHubTracker}>
            <div className="githubTrackerIntro">
              <span className="githubTrackerIntroIcon" aria-hidden="true"><GitHubIcon /></span>
              <div><strong>Активность профиля</strong><span>Трекер повторяет календарь contributions из публичного профиля GitHub. Пароль и GitHub token не требуются.</span></div>
            </div>
            <label className="field">
              <span className="fieldLabel">GitHub username</span>
              <input
                className="input"
                value={draftGitHubUsername}
                maxLength={39}
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                placeholder="octocat"
                aria-invalid={draftGitHubUsername.length > 0 && !githubUsernameIsValid(draftGitHubUsername)}
                onChange={(event) => setDraftGitHubUsername(event.target.value)}
              />
            </label>
            {githubData?.configured && (
              <div className="githubTrackerSummary">
                <div className="githubTrackerSummaryValue"><strong>{githubContributionValue}</strong><span>{githubPeriodCaption}</span></div>
                <GitHubPeriodSelector value={githubPeriod} onChange={setGitHubPeriod} />
                <GitHubHeatmap days={githubData.days} expanded weeks={githubPeriod === "year" ? GITHUB_TRACKER_WEEKS : GITHUB_TRACKER_CARD_WEEKS} />
              </div>
            )}
            <p className="githubTrackerHint">Счётчик берётся из календаря профиля GitHub. GitHub может включать в него другую активность и обезличенные данные приватных репозиториев.</p>
            {githubError && <div className="formError" role="alert">{githubError}</div>}
            {githubData?.configured && <button type="button" className="secondaryButton githubTrackerRefresh" disabled={githubLoading || githubSaving} onClick={() => void refreshGitHubTracker()}>{githubLoading ? "Обновление…" : "Обновить статистику"}</button>}
            <div className="modalActions">
              {githubData?.configured && <button type="button" className="dangerButton" disabled={githubSaving} onClick={() => void deleteGitHubTracker()}>Отключить</button>}
              <button className="primaryButton" disabled={githubSaving || !githubUsernameIsValid(draftGitHubUsername)}>{githubSaving ? "Сохранение…" : "Сохранить"}</button>
            </div>
          </form>
        </Modal>
      )}


      {(trackerModal === "custom-create" || trackerModal === "custom-edit") && (
        <Modal title={trackerModal === "custom-edit" ? "Изменить трекер" : "Новый трекер"} onClose={() => { if (!customSaving) { setTrackerError(null); setTrackerModal(trackerModal === "custom-edit" ? "custom" : "picker"); } }}>
          <form className="customTrackerForm" noValidate onSubmit={saveCustomTracker}>
            <label className="field"><span className="fieldLabel">Название</span><input className="input" value={customName} maxLength={40} autoFocus placeholder="Например, прочитать книг" onChange={(event) => setCustomName(event.target.value)} /></label>
            <div className="customTrackerValueGrid">
              <label className="field"><span className="fieldLabel">Итоговая цель</span><input className="input" type="number" min="0.001" max="1000000000" step="any" inputMode="decimal" value={customTarget} onChange={(event) => setCustomTarget(event.target.value)} /></label>
              <label className="field"><span className="fieldLabel">Шаг</span><input className="input" type="number" min="0.001" max="1000000000" step="any" inputMode="decimal" value={customStep} onChange={(event) => setCustomStep(event.target.value)} /></label>
            </div>
            <fieldset className="customTrackerIconPicker">
              <legend>Иконка</legend>
              <div className="customTrackerIconGrid">
                {CUSTOM_TRACKER_ICONS.map(([id, src], index) => (
                  <button type="button" key={id} className={customIcon === id ? "isSelected" : ""} onClick={() => setCustomIcon(id)} aria-pressed={customIcon === id} aria-label={`Выбрать иконку ${index + 1} из ${CUSTOM_TRACKER_ICONS.length}`}>
                    <img src={src} alt="" aria-hidden="true" draggable={false} />
                  </button>
                ))}
              </div>
            </fieldset>
            {trackerError && <div className="formError" role="alert">{trackerError}</div>}
            <div className="modalActions"><button type="button" className="secondaryButton" disabled={customSaving} onClick={() => { setTrackerError(null); setTrackerModal(trackerModal === "custom-edit" ? "custom" : "picker"); }}>{trackerModal === "custom-edit" ? "Отмена" : "Назад"}</button><button className="primaryButton" disabled={customSaving || !customName.trim() || Number(customTarget.replace(",", ".")) <= 0 || Number(customStep.replace(",", ".")) <= 0 || Number(customStep.replace(",", ".")) > Number(customTarget.replace(",", "."))}>{customSaving ? "Сохранение…" : trackerModal === "custom-edit" ? "Сохранить" : "Создать"}</button></div>
          </form>
        </Modal>
      )}

      {trackerModal === "custom" && activeCustomTracker && (
        <Modal title={activeCustomTracker.name} onClose={() => setTrackerModal(null)}>
          <div className="customTrackerDetailActions">
            <button type="button" className="secondaryButton customTrackerEditButton" disabled={customSaving} onClick={() => openCustomTrackerEditor(activeCustomTracker)} aria-label={`Изменить трекер «${activeCustomTracker.name}»`}>
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="m16 3 5 5L8 21H3v-5L16 3Z" /><path d="m13 6 5 5" /></svg>
              Изменить
            </button>
          </div>
          <div className="customTrackerDetail">
            <span className="customTrackerDetailIcon" aria-hidden="true"><CustomTrackerIcon icon={activeCustomTracker.icon} /></span>
            <div><strong>{formatTrackerNumber(activeCustomTracker.currentValue)}<small> / {formatTrackerNumber(activeCustomTracker.targetValue)}</small></strong><span>Шаг: {formatTrackerNumber(activeCustomTracker.stepValue)}</span></div>
          </div>
          <div className="customTrackerProgress"><span style={{ width: `${Math.min(100, activeCustomTracker.currentValue / activeCustomTracker.targetValue * 100)}%` }} /></div>
          <div className="customTrackerStepControls">
            <button type="button" onClick={() => void stepCustomTracker(activeCustomTracker, -1)} disabled={customSaving || activeCustomTracker.currentValue <= 0}>− {formatTrackerNumber(activeCustomTracker.stepValue)}</button>
            <button type="button" onClick={() => void stepCustomTracker(activeCustomTracker, 1)} disabled={customSaving || activeCustomTracker.currentValue >= activeCustomTracker.targetValue}>＋ {formatTrackerNumber(activeCustomTracker.stepValue)}</button>
          </div>
          {trackerError && <div className="formError" role="alert">{trackerError}</div>}
          <section className="trackerStatistics customTrackerDetailStatistics" aria-label="Статистика трекера">
            <header className="trackerStatisticsHeader">
              <div><strong>Статистика</strong></div>
              <div className="trackerStatisticsPeriod" role="group" aria-label="Период статистики трекера">
                <button type="button" className={customStatisticsPeriod === 7 ? "active" : ""} aria-pressed={customStatisticsPeriod === 7} onClick={() => setCustomStatisticsPeriod(7)}>7 дней</button>
                <button type="button" className={customStatisticsPeriod === 30 ? "active" : ""} aria-pressed={customStatisticsPeriod === 30} onClick={() => setCustomStatisticsPeriod(30)}>30 дней</button>
              </div>
            </header>
            <CustomTrackerStatisticsCard
              tracker={activeCustomTracker}
              entries={(trackerData?.customHistory ?? []).filter((entry) => entry.trackerId === activeCustomTracker.id).sort((left, right) => left.date.localeCompare(right.date))}
              currentDate={trackerDate}
              period={customStatisticsPeriod}
            />
          </section>
          <div className="modalActions"><button type="button" className="dangerButton" disabled={customSaving} onClick={() => void deleteCustomTracker(activeCustomTracker)}>Удалить</button><button type="button" className="primaryButton" onClick={() => setTrackerModal(null)}>Готово</button></div>
        </Modal>
      )}

      {(trackerModal === "calories" || standaloneCalories) && (
        <CaloriesSurface standalone={standaloneCalories} onClose={() => { closeNutritionPicker(); setTrackerModal(null); }}>
          {standaloneCalories && <CaloriesWeekPicker value={trackerDate} onChange={setTrackerDate} />}
          {fatSecretNotice && (
            <div className={`fatSecretNotice fatSecretNotice-${fatSecretNotice.tone}`} role={fatSecretNotice.tone === "error" ? "alert" : "status"}>
              {fatSecretNotice.text}
            </div>
          )}
          {calorieMode === "fatsecret" && fatSecretStatus === null ? (
            <div className="nutritionSync" role="status">Проверяем подключение FatSecret…</div>
          ) : (
            <>
              <div className="nutritionHero">
                <div className="nutritionHeroSummary">
                  <div className="nutritionHeroReading">
                    <span>Калории</span>
                    <strong>{nutrition ? formatNutrition(nutrition.calories, 0) : "—"}</strong>
                    <small>/ {formatNutrition(calorieGoal, 0)} ккал</small>
                  </div>
                  <div className="nutritionGoalRing" aria-label={`Выполнено ${Math.round(calorieProgress)}% дневной нормы`}>
                    <svg viewBox="0 0 64 64" aria-hidden="true">
                      <circle cx="32" cy="32" r="26" pathLength="100" />
                      <circle className="nutritionGoalRingValue" cx="32" cy="32" r="26" pathLength="100" strokeDasharray="100" strokeDashoffset={100 - calorieProgress} />
                    </svg>
                    <strong>{Math.round(calorieProgress)}%</strong>
                  </div>
                </div>
                <div className="nutritionGoalProgress" aria-label={`Выполнено ${nutrition ? Math.round(Math.min(100, nutrition.calories / calorieGoal * 100)) : 0}% дневной нормы`}>
                  <span style={{ width: `${calorieProgress}%` }} />
                </div>
                <div className="nutritionGoalStatus">
                  <span>{nutrition ? (nutrition.calories <= calorieGoal
                    ? `Осталось: ${formatNutrition(calorieGoal - nutrition.calories, 0)} ккал`
                    : `Сверх нормы ${formatNutrition(nutrition.calories - calorieGoal, 0)} ккал`) : ""}</span>
                  <button type="button" className="nutritionGoalEdit" onClick={openNutritionGoals} aria-label="Изменить дневные нормы">
                    Цель: {formatNutrition(calorieGoal, 0)} ккал <span aria-hidden="true">✎</span>
                  </button>
                </div>
              </div>

              <div className="macroGrid" aria-label="КБЖУ за день">
                <div><span>Белки</span><strong>{nutrition ? formatNutrition(nutrition.protein) : "—"}<small> г</small></strong><small>/ {proteinGoal} г</small><div className="macroProgress" aria-label={`Белки: ${Math.round(macroProgress.protein)}% цели`}><i style={{ width: `${macroProgress.protein}%` }} /></div></div>
                <div><span>Жиры</span><strong>{nutrition ? formatNutrition(nutrition.fat) : "—"}<small> г</small></strong><small>/ {fatGoal} г</small><div className="macroProgress" aria-label={`Жиры: ${Math.round(macroProgress.fat)}% цели`}><i style={{ width: `${macroProgress.fat}%` }} /></div></div>
                <div><span>Углеводы</span><strong>{nutrition ? formatNutrition(nutrition.carbohydrate) : "—"}<small> г</small></strong><small>/ {carbohydrateGoal} г</small><div className="macroProgress" aria-label={`Углеводы: ${Math.round(macroProgress.carbohydrate)}% цели`}><i style={{ width: `${macroProgress.carbohydrate}%` }} /></div></div>
              </div>

              <section className="nutritionMeals" aria-labelledby="nutrition-meals-title">
                <div className="nutritionSectionTitle">
                  <div><strong id="nutrition-meals-title">Приёмы пищи</strong></div>
                </div>
                {FATSECRET_MEALS.map((definition) => {
                  const meal = nutrition?.meals.find((item) => item.meal === definition.apiName);
                  const entries = meal?.entries ?? [];
                  const expanded = expandedNutritionMeals.has(definition.id);
                  const entryListExpanded = expandedNutritionEntryLists.has(definition.id);
                  const visibleEntries = entryListExpanded ? entries : entries.slice(0, 3);
                  return (
                    <article className={`nutritionMealCard nutritionMeal-${definition.id}${expanded ? " isExpanded" : ""}`} key={definition.id}>
                      <div className="nutritionMealHeader">
                        <button type="button" className="nutritionMealToggle" onClick={() => toggleNutritionMeal(definition.id)} aria-expanded={expanded}>
                          <span><strong>{definition.label}</strong></span>
                        </button>
                        <strong className="nutritionMealCalories">{formatNutrition(meal?.calories ?? 0, 0)}<small> ккал</small></strong>
                        <button type="button" className="nutritionMealAdd" onClick={() => openNutritionPicker(definition.id)} aria-label={`Добавить в ${definition.label}`}>＋</button>
                        <button type="button" className="nutritionMealChevron" disabled={entries.length === 0} onClick={() => toggleNutritionMeal(definition.id)} aria-label={expanded ? `Свернуть ${definition.label}` : `Развернуть ${definition.label}`}>{expanded ? "⌃" : "⌄"}</button>
                      </div>
                      {expanded && entries.length > 0 && (
                        <div className="nutritionMealBody">
                          <div className="nutritionEntries">
                            {visibleEntries.map((entry) => (
                              <div className="nutritionEntry" key={entry.id}>
                                <div className="nutritionEntryCopy">
                                  <strong>{entry.name || entry.description}</strong>
                                  <span className="nutritionEntryServing">{nutritionEntryServingLabel(entry)}</span>
                                  <span className="nutritionEntryMacros" aria-label={`Белки ${formatNutrition(entry.protein)} грамм, жиры ${formatNutrition(entry.fat)} грамм, углеводы ${formatNutrition(entry.carbohydrate)} грамм`}>
                                    <span>Б {formatNutrition(entry.protein)}</span>
                                    <span>Ж {formatNutrition(entry.fat)}</span>
                                    <span>У {formatNutrition(entry.carbohydrate)}</span>
                                  </span>
                                </div>
                                <div className="nutritionEntryAside">
                                  <strong className="nutritionEntryCalories"><span>{formatNutrition(entry.calories, 0)}</span><small>ккал</small></strong>
                                  <button type="button" className="nutritionEntryMore" onClick={() => setNutritionEntryMenu((current) => current === entry.id ? null : entry.id)} aria-label={`Действия: ${entry.name}`}>•••</button>
                                </div>
                                {nutritionEntryMenu === entry.id && (
                                  <div className="nutritionEntryMenu">
                                    <button type="button" onClick={() => openNutritionPicker(definition.id, entry)}>Изменить порцию</button>
                                    <button type="button" disabled={nutritionMutationBusy} onClick={() => void deleteNutritionEntry(entry)}>Удалить</button>
                                  </div>
                                )}
                              </div>
                            ))}
                            {entries.length > 3 && <button type="button" className="nutritionEntriesMore" onClick={() => toggleNutritionEntryList(definition.id)}>{entryListExpanded ? "Скрыть" : "Показать больше"}<span aria-hidden="true">{entryListExpanded ? "⌃" : "⌄"}</span></button>}
                            <div className="nutritionMealTotals" aria-label={`Итого за ${definition.label.toLocaleLowerCase("ru-RU")}`}>
                              <strong>Итого за приём</strong>
                              <div className="nutritionMealTotalMacros">
                                <span>Б {formatNutrition(meal?.protein ?? 0)} г</span>
                                <span>Ж {formatNutrition(meal?.fat ?? 0)} г</span>
                                <span>У {formatNutrition(meal?.carbohydrate ?? 0)} г</span>
                              </div>
                            </div>
                          </div>
                        </div>
                      )}
                    </article>
                  );
                })}
              </section>

              <section className={`calorieReport${calorieReportExpanded ? " isExpanded" : ""}`} aria-labelledby="calorie-report-title">
                <button type="button" className="calorieReportToggle" aria-expanded={calorieReportExpanded} aria-controls="calorie-report-options" onClick={() => setCalorieReportExpanded((current) => !current)}>
                  <strong id="calorie-report-title">Детальный отчёт КБЖУ</strong>
                  <span className="calorieReportBadge">PDF</span>
                  <span className="calorieReportChevron" aria-hidden="true">{calorieReportExpanded ? "⌃" : "⌄"}</span>
                </button>
                {calorieReportExpanded && <div className="calorieReportBody" id="calorie-report-options">
                  <p>PDF с итогами по дням и разбивкой на завтрак, обед, ужин и перекусы.</p>
                  <div className="calorieReportPresets" role="group" aria-label="Быстрый выбор периода отчёта">
                    <button type="button" onClick={() => { setCalorieReportFrom(addDaysToDateKey(trackerDate, -6)); setCalorieReportTo(trackerDate); setCalorieReportError(null); }}>7 дней</button>
                    <button type="button" onClick={() => { setCalorieReportFrom(addDaysToDateKey(trackerDate, -29)); setCalorieReportTo(trackerDate); setCalorieReportError(null); }}>30 дней</button>
                  </div>
                  <div className="calorieReportDates">
                    <label><span>С</span><input type="date" max={calorieReportTo || trackerDate} value={calorieReportFrom} onInput={(event) => setCalorieReportFrom(event.currentTarget.value)} onChange={(event) => setCalorieReportFrom(event.currentTarget.value)} /></label>
                    <label><span>По</span><input type="date" min={calorieReportFrom} max={trackerDate} value={calorieReportTo} onInput={(event) => setCalorieReportTo(event.currentTarget.value)} onChange={(event) => setCalorieReportTo(event.currentTarget.value)} /></label>
                  </div>
                  <button type="button" className="primaryButton calorieReportDownload" disabled={calorieReportBusy} aria-busy={calorieReportBusy} onClick={() => void createCalorieReport()}>{calorieReportBusy ? "Собираем отчёт…" : "Скачать детальный PDF"}</button>
                  <small>За один раз можно выгрузить до 31 дня. Данные загружаются из выбранного дневника питания.</small>
                  {calorieReportError && <div className="formError" role="alert">{calorieReportError}</div>}
                </div>}
              </section>

              {nutritionLoading && <div className="nutritionSync" role="status">Обновляем дневник…</div>}
              {nutritionError && !nutritionPickerMeal && <div className="formError" role="alert">{nutritionError}</div>}
              {calorieMode === "fatsecret" && <div className="modalActions fatSecretActions">
                <button type="button" className="secondaryButton" disabled={nutritionLoading || fatSecretDisconnecting} onClick={() => void disconnectFatSecret()}>Отключить</button>
              </div>}
            </>
          )}
          {calorieMode === "fatsecret" && <a className="fatSecretAttribution fatSecretAttributionBottom" href="https://platform.fatsecret.com" target="_blank" rel="noreferrer">Powered by fatsecret Platform API</a>}
        </CaloriesSurface>
      )}

      {nutritionGoalsOpen && (
        <Modal title="Дневные нормы" className="nutritionGoalsModal" onClose={() => { if (!nutritionGoalsSaving) setNutritionGoalsOpen(false); }}>
          <div className="nutritionGoalsIntro">
            <strong>Настройте свой ориентир</strong>
            <p>Рассчитайте нормы по параметрам тела или задайте БЖУ самостоятельно.</p>
          </div>
          <div className="nutritionGoalsMode" role="tablist" aria-label="Способ настройки норм">
            <button type="button" role="tab" aria-selected={nutritionGoalsMode === "quick"} className={nutritionGoalsMode === "quick" ? "isActive" : ""} onClick={() => { setNutritionGoalsMode("quick"); setNutritionGoalsError(null); }}>Быстрый расчёт</button>
            <button type="button" role="tab" aria-selected={nutritionGoalsMode === "manual"} className={nutritionGoalsMode === "manual" ? "isActive" : ""} onClick={() => { setNutritionGoalsMode("manual"); setNutritionGoalsError(null); }}>Вручную</button>
          </div>

          {nutritionGoalsMode === "quick" ? (
            <div className="nutritionCalculatorPanel" role="tabpanel">
              <div className="nutritionCalculatorSection">
                <span className="nutritionCalculatorLabel">Пол для расчёта</span>
                <div className="nutritionChoiceGroup nutritionChoiceGroup-two">
                  <button type="button" className={nutritionCalculator.sex === "female" ? "isSelected" : ""} onClick={() => setNutritionCalculator((current) => ({ ...current, sex: "female" }))}>Женский</button>
                  <button type="button" className={nutritionCalculator.sex === "male" ? "isSelected" : ""} onClick={() => setNutritionCalculator((current) => ({ ...current, sex: "male" }))}>Мужской</button>
                </div>
              </div>
              <div className="nutritionCalculatorNumbers">
                <label><span>Возраст</span><span><input type="number" min="18" max="100" inputMode="numeric" value={nutritionCalculator.age} onChange={(event) => setNutritionCalculator((current) => ({ ...current, age: event.target.value }))} /><em>лет</em></span></label>
                <label><span>Рост</span><span><input type="number" min="120" max="230" inputMode="numeric" value={nutritionCalculator.heightCm} onChange={(event) => setNutritionCalculator((current) => ({ ...current, heightCm: event.target.value }))} /><em>см</em></span></label>
                <label><span>Вес</span><span><input type="number" min="35" max="300" step="0.1" inputMode="decimal" value={nutritionCalculator.weightKg} onChange={(event) => setNutritionCalculator((current) => ({ ...current, weightKg: event.target.value }))} /><em>кг</em></span></label>
              </div>
              <label className="nutritionCalculatorSelect">
                <span><strong>Активность</strong><small>Ваш обычный недельный ритм</small></span>
                <select value={nutritionCalculator.activity} onChange={(event) => setNutritionCalculator((current) => ({ ...current, activity: event.target.value as NutritionCalculatorActivity }))}>
                  <option value="minimal">Минимальная</option>
                  <option value="light">Лёгкая · 1–3 тренировки</option>
                  <option value="moderate">Средняя · 3–5 тренировок</option>
                  <option value="high">Высокая · 6–7 тренировок</option>
                </select>
              </label>
              <div className="nutritionCalculatorSection">
                <span className="nutritionCalculatorLabel">Цель</span>
                <div className="nutritionChoiceGroup nutritionChoiceGroup-three">
                  <button type="button" className={nutritionCalculator.goal === "lose" ? "isSelected" : ""} onClick={() => setNutritionCalculator((current) => ({ ...current, goal: "lose" }))}>Похудение</button>
                  <button type="button" className={nutritionCalculator.goal === "maintain" ? "isSelected" : ""} onClick={() => setNutritionCalculator((current) => ({ ...current, goal: "maintain" }))}>Поддержание</button>
                  <button type="button" className={nutritionCalculator.goal === "gain" ? "isSelected" : ""} onClick={() => setNutritionCalculator((current) => ({ ...current, goal: "gain" }))}>Набор мышц</button>
                </div>
              </div>
              {nutritionCalculator.goal !== "maintain" && (
                <div className="nutritionCalculatorSection nutritionCalculatorPace">
                  <span className="nutritionCalculatorLabel">Темп</span>
                  <div className="nutritionChoiceGroup nutritionChoiceGroup-two">
                    <button type="button" className={nutritionCalculator.pace === "gentle" ? "isSelected" : ""} onClick={() => setNutritionCalculator((current) => ({ ...current, pace: "gentle" }))}>Плавный</button>
                    <button type="button" className={nutritionCalculator.pace === "standard" ? "isSelected" : ""} onClick={() => setNutritionCalculator((current) => ({ ...current, pace: "standard" }))}>Стандартный</button>
                  </div>
                </div>
              )}
              {nutritionCalculator.goal === "gain" && <p className="nutritionCalculatorNote">Набор мышц с небольшим профицитом: {nutritionCalculator.pace === "gentle" ? "5%" : "10%"} к поддержанию, белок — 2 г/кг. Расчёт предполагает силовые тренировки. Следите за весом и талией и корректируйте норму по динамике за 2–3 недели: полностью исключить набор жира нельзя.</p>}
              {calculatedNutritionGoals ? (
                <div className="nutritionGoalsResult" aria-live="polite">
                  <div><span>Калории</span><strong>{formatNutrition(calculatedNutritionGoals.calorieGoal, 0)}</strong><small>ккал</small></div>
                  <div><span>Белки</span><strong>{calculatedNutritionGoals.proteinGoal}</strong><small>г</small></div>
                  <div><span>Жиры</span><strong>{calculatedNutritionGoals.fatGoal}</strong><small>г</small></div>
                  <div><span>Углеводы</span><strong>{calculatedNutritionGoals.carbohydrateGoal}</strong><small>г</small></div>
                </div>
              ) : <div className="nutritionGoalsHint" role="alert">Заполните возраст 18–100 лет, рост 120–230 см и вес 35–300 кг.</div>}
              <p className="nutritionCalculatorNote">Расчёт ориентировочный и предназначен для взрослых. При медицинских ограничениях обсудите нормы со специалистом.</p>
            </div>
          ) : (
            <div className="nutritionManualPanel" role="tabpanel">
              <p>Введите БЖУ — калории пересчитаются автоматически по формуле 4 · 9 · 4.</p>
              <div className="nutritionManualFields">
                <label><span>Белки</span><span className="nutritionGoalInput"><input type="number" min="1" max="1000" step="5" inputMode="numeric" value={draftNutritionGoals.proteinGoal} onChange={(event) => setDraftNutritionGoals((current) => ({ ...current, proteinGoal: Number(event.target.value) }))} /><em>г</em></span></label>
                <label><span>Жиры</span><span className="nutritionGoalInput"><input type="number" min="1" max="1000" step="5" inputMode="numeric" value={draftNutritionGoals.fatGoal} onChange={(event) => setDraftNutritionGoals((current) => ({ ...current, fatGoal: Number(event.target.value) }))} /><em>г</em></span></label>
                <label><span>Углеводы</span><span className="nutritionGoalInput"><input type="number" min="1" max="1000" step="5" inputMode="numeric" value={draftNutritionGoals.carbohydrateGoal} onChange={(event) => setDraftNutritionGoals((current) => ({ ...current, carbohydrateGoal: Number(event.target.value) }))} /><em>г</em></span></label>
              </div>
              <div className="nutritionManualCalories" aria-live="polite"><span><strong>Калории</strong><small>Белки × 4 + жиры × 9 + углеводы × 4</small></span><strong>{formatNutrition(manualNutritionGoals.calorieGoal, 0)} <small>ккал</small></strong></div>
              {!nutritionGoalsAreValid && <div className="nutritionGoalsHint" role="alert">Каждое значение БЖУ должно быть от 1 до 1 000 г, итог — от 500 до 10 000 ккал.</div>}
            </div>
          )}
          {nutritionGoalsError && <div className="formError" role="alert">{nutritionGoalsError}</div>}
          <button type="button" className="primaryButton nutritionGoalsSave" disabled={!trackerReady || !nutritionGoalsAreValid || !nutritionGoalsChanged || nutritionGoalsSaving} onClick={() => void saveNutritionGoals()}>
            {nutritionGoalsSaving ? "Сохраняем…" : nutritionGoalsChanged ? "Сохранить нормы" : "Нормы сохранены"}
          </button>
        </Modal>
      )}

      {nutritionPickerMeal && (
        <FoodPicker
          meal={nutritionPickerMeal}
          date={trackerDate}
          entry={nutritionEditingEntry}
          externalError={nutritionError}
          onClose={closeNutritionPicker}
          onSaved={acceptNutritionMutation}
        />
      )}

      {trackerModal === "water" && (
        <Modal title="Вода" onClose={() => setTrackerModal(null)}>
          <div className="waterTrackerSummary">
            <div><span>Сегодня выпито</span><strong>{draftWaterCount}<small> / {draftWaterGoal}</small></strong></div>
            <div className="waterTrackerProgress" aria-label={`Выполнено ${Math.round(Math.min(100, draftWaterCount / draftWaterGoal * 100))}%`}><span style={{ width: `${Math.min(100, draftWaterCount / draftWaterGoal * 100)}%` }} /></div>
          </div>
          <TrackerStepper
            label="Выпито сегодня"
            value={draftWaterCount}
            suffix="стаканов"
            decrementDisabled={draftWaterCount <= 0}
            incrementDisabled={draftWaterCount >= 99}
            onDecrease={() => setDraftWaterCount((value) => Math.max(0, value - 1))}
            onIncrease={() => setDraftWaterCount((value) => Math.min(99, value + 1))}
          />
          <TrackerStepper
            label="Дневная норма"
            value={draftWaterGoal}
            suffix="стаканов"
            detail={`≈ ${(draftWaterGoal * WATER_GLASS_ML).toLocaleString("ru-RU")} мл`}
            decrementDisabled={draftWaterGoal <= 1}
            incrementDisabled={draftWaterGoal >= 30}
            onDecrease={() => setDraftWaterGoal((value) => Math.max(1, value - 1))}
            onIncrease={() => setDraftWaterGoal((value) => Math.min(30, value + 1))}
          />
          <p className="waterTrackerHint">Расчёт: 1 стакан ≈ {WATER_GLASS_ML} мл. Учёт ведётся отдельно для каждого дня.</p>
          {trackerError && <div className="formError" role="alert">{trackerError}</div>}
          <div className="modalActions">
            <button type="button" className="secondaryButton" disabled={waterSaving} onClick={() => setTrackerModal(null)}>Отмена</button>
            <button type="button" className="primaryButton" disabled={waterSaving} aria-busy={waterSaving} onClick={() => void saveWaterTracker()}>{waterSaving ? "Сохранение…" : "Сохранить"}</button>
          </div>
        </Modal>
      )}

      {trackerModal === "weight" && (
        <Modal title="Вес" onClose={() => setTrackerModal(null)}>
          <div className="weightTrackerEditor">
            <label htmlFor="tracker-weight-input">Текущий вес</label>
            <div className="weightTrackerInputRow">
              <input
                id="tracker-weight-input"
                value={draftWeight}
                inputMode="decimal"
                autoComplete="off"
                autoFocus
                aria-invalid={!weightIsValid}
                onChange={(event) => setDraftWeight(event.target.value)}
              />
              <span>кг</span>
            </div>
            <small>От 20 до 500 кг. Можно указать десятые через запятую.</small>
          </div>
          {trackerError && <div className="formError" role="alert">{trackerError}</div>}
          <div className="modalActions">
            <button type="button" className="secondaryButton" disabled={weightSaving} onClick={() => setTrackerModal(null)}>Отмена</button>
            <button type="button" className="primaryButton" disabled={!weightIsValid || weightSaving} aria-busy={weightSaving} onClick={() => void saveWeightTracker()}>{weightSaving ? "Сохранение…" : "Сохранить"}</button>
          </div>
        </Modal>
      )}
    </>
  );
}

function FoodPicker({
  meal,
  date,
  entry,
  externalError,
  onClose,
  onSaved,
}: {
  meal: FatSecretMeal;
  date: string;
  entry: FatSecretNutritionEntry | null;
  externalError?: string | null;
  onClose: () => void;
  onSaved: (nutrition: FatSecretNutrition) => void;
}) {
  const mealMeta = FATSECRET_MEALS.find((item) => item.id === meal) ?? FATSECRET_MEALS[3];
  const [pickerTab, setPickerTab] = useState<"recent" | "search">("recent");
  const [recentScope, setRecentScope] = useState<"meal" | "all">("meal");
  const [query, setQuery] = useState("");
  const [correctedQuery, setCorrectedQuery] = useState("");
  const [searchFoods, setSearchFoods] = useState<FatSecretFood[]>([]);
  const [recentFoods, setRecentFoods] = useState<FatSecretFood[]>([]);
  const [searchPage, setSearchPage] = useState(0);
  const [searchHasMore, setSearchHasMore] = useState(false);
  const [externalSearchUnavailable, setExternalSearchUnavailable] = useState(false);
  const [queryComposing, setQueryComposing] = useState(false);
  const [visibleFoodCount, setVisibleFoodCount] = useState(20);
  const [selectedFood, setSelectedFood] = useState<FatSecretFood | null>(null);
  const [selectedFromRecentFallback, setSelectedFromRecentFallback] = useState(false);
  const [servingID, setServingID] = useState(entry?.servingId ?? "");
  const [units, setUnits] = useState(() => entry?.numberOfUnits ? String(entry.numberOfUnits) : "1");
  const [barcode, setBarcode] = useState("");
  const [loading, setLoading] = useState(false);
  const [recentLoading, setRecentLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deletingFood, setDeletingFood] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createMode, setCreateMode] = useState(false);
  const [correctionMode, setCorrectionMode] = useState(false);
  const [barcodeScannerOpen, setBarcodeScannerOpen] = useState(false);
  const [nutritionScannerOpen, setNutritionScannerOpen] = useState(false);
  const [draftName, setDraftName] = useState("");
  const [draftBrand, setDraftBrand] = useState("");
  const [draftCalories, setDraftCalories] = useState("");
  const [draftProtein, setDraftProtein] = useState("");
  const [draftFat, setDraftFat] = useState("");
  const [draftCarbs, setDraftCarbs] = useState("");
  const [draftNutritionUnit, setDraftNutritionUnit] = useState<"g" | "ml">("g");
  const [draftPortionAmount, setDraftPortionAmount] = useState("");
  const [foodCandidates, setFoodCandidates] = useState<LocalFood[]>([]);
  const [foodCandidatesLoading, setFoodCandidatesLoading] = useState(false);
  const [foodCandidatesError, setFoodCandidatesError] = useState("");
  const [candidateLinking, setCandidateLinking] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const searchAbortRef = useRef<AbortController | null>(null);
  const searchRequestIDRef = useRef(0);

  const selectedServing = selectedFood?.servings?.find((serving) => serving.id === servingID) ?? null;
  const portionVariants = (selectedFood?.servings ?? []).filter((serving) => serving.numberOfUnits === 1
    && Boolean(serving.metricAmount && serving.metricAmount > 0)
    && (serving.metricUnit === "g" || serving.metricUnit === "ml"));
  const parsedUnits = Number(units.replace(",", "."));
  const unitsValid = Number.isFinite(parsedUnits) && parsedUnits > 0 && parsedUnits <= 10000;
  const multiplier = selectedServing && selectedServing.numberOfUnits > 0 && unitsValid
    ? parsedUnits / selectedServing.numberOfUnits
    : 0;
  const mergedFoods = useMemo(
    () => mergeFatSecretFoodResults(searchFoods, recentFoods, query, correctedQuery),
    [searchFoods, recentFoods, query, correctedQuery],
  );
  const visibleFoods = mergedFoods.slice(0, visibleFoodCount);
  const canShowMoreFoods = searchHasMore || visibleFoodCount < mergedFoods.length;
  const candidateMacroValues = [draftCalories, draftProtein, draftFat, draftCarbs]
    .map((value) => value.trim() === "" ? Number.NaN : Number(value.replace(",", ".")));
  const foodCandidateSearchReady = createMode && !correctionMode && Boolean(barcode)
    && candidateMacroValues.every((value) => Number.isFinite(value) && value >= 0)
    && candidateMacroValues[0] <= 1000
    && candidateMacroValues.slice(1).every((value) => value <= 100);
  const parsedPortionAmount = draftPortionAmount.trim() === "" ? 0 : Number(draftPortionAmount.replace(",", "."));
  const portionAmountIsValid = draftPortionAmount.trim() === "" || (Number.isFinite(parsedPortionAmount) && parsedPortionAmount >= 0.1 && parsedPortionAmount <= 10000);
  const nutritionUnitLabel = draftNutritionUnit === "ml" ? "мл" : "г";
  const visibleError = error || externalError;
  const selectedCatalogFood = selectedFood as (FatSecretFood & Partial<LocalFood>) | null;
  const canDeleteSelectedFood = selectedCatalogFood?.provider === "local";

  useEffect(() => {
    if (entry) {
      setRecentLoading(false);
      return;
    }
    let active = true;
    setRecentLoading(true);
    setError(null);
    api.recentLocalFoods(recentScope === "meal" ? meal : undefined)
      .then((foods) => { if (active) setRecentFoods(foods); })
      .catch((cause: unknown) => { if (active) setError(cause instanceof Error ? cause.message : "Не удалось загрузить последнюю еду"); })
      .finally(() => { if (active) setRecentLoading(false); });
    return () => { active = false; };
  }, [entry, meal, recentScope]);

  useEffect(() => {
    if (!foodCandidateSearchReady) {
      setFoodCandidates([]);
      setFoodCandidatesLoading(false);
      setFoodCandidatesError("");
      return;
    }
    const values = candidateMacroValues;
    let active = true;
    setFoodCandidates([]);
    setFoodCandidatesError("");
    setFoodCandidatesLoading(true);
    const timer = window.setTimeout(() => {
      api.localFoodCandidates({
        name: draftName.trim(),
        brandName: draftBrand.trim(),
        caloriesPer100g: values[0],
        proteinPer100g: values[1],
        fatPer100g: values[2],
        carbohydratePer100g: values[3],
        nutritionUnit: draftNutritionUnit,
      })
        .then((items) => { if (active) setFoodCandidates(items); })
        .catch((cause: unknown) => {
          if (!active) return;
          setFoodCandidates([]);
          setFoodCandidatesError(cause instanceof Error ? cause.message : "Не удалось проверить похожие продукты");
        })
        .finally(() => { if (active) setFoodCandidatesLoading(false); });
    }, 160);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [barcode, correctionMode, createMode, draftBrand, draftCalories, draftCarbs, draftFat, draftName, draftNutritionUnit, draftProtein, foodCandidateSearchReady]);

  useEffect(() => {
    if (!entry) return;
    let active = true;
    setLoading(true);
    api.localFood(entry.foodId)
      .then((food) => {
        if (!active) return;
        setSelectedFood(food);
        setServingID(entry.servingId);
        setUnits(String(entry.numberOfUnits));
      })
      .catch((cause: unknown) => { if (active) setError(cause instanceof Error ? cause.message : "Не удалось загрузить продукт"); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [entry]);

  useEffect(() => {
    if (pickerTab !== "search") return;
    const normalized = query.trim();
    if (normalized.length < 3) {
      searchRequestIDRef.current += 1;
      searchAbortRef.current?.abort();
      setLoading(false);
      setLoadingMore(false);
      setSearchFoods([]);
      setCorrectedQuery("");
      setSearchPage(0);
      setSearchHasMore(false);
      setExternalSearchUnavailable(false);
      setVisibleFoodCount(20);
      return;
    }
    if (queryComposing) return;
    let active = true;
    setLoading(false);
    setLoadingMore(false);
    const controller = new AbortController();
    searchAbortRef.current?.abort();
    searchAbortRef.current = controller;
    const requestID = ++searchRequestIDRef.current;
    const timer = window.setTimeout(() => {
      setLoading(true);
      setError(null);
      api.searchCatalogFoods(normalized, { signal: controller.signal })
        .then((result) => {
          if (!active || requestID !== searchRequestIDRef.current) return;
          setSearchFoods(result.foods);
          setCorrectedQuery(result.correctedQuery ?? "");
          setSearchPage(result.page);
          setSearchHasMore(result.hasMore);
          setExternalSearchUnavailable(Boolean(result.externalUnavailable));
          setVisibleFoodCount(20);
        })
        .catch((cause: unknown) => {
          if (!active || controller.signal.aborted || requestID !== searchRequestIDRef.current) return;
          setSearchFoods([]);
          setSearchPage(0);
          setSearchHasMore(false);
          setExternalSearchUnavailable(false);
          setVisibleFoodCount(20);
          setError(cause instanceof Error ? cause.message : "Не удалось выполнить поиск");
        })
        .finally(() => { if (active && requestID === searchRequestIDRef.current) setLoading(false); });
    }, 300);
    return () => {
      active = false;
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [pickerTab, query, queryComposing]);

  async function searchExactlyAsTyped() {
    const normalized = query.trim();
    if (normalized.length < 3) return;
    const controller = new AbortController();
    searchAbortRef.current?.abort();
    searchAbortRef.current = controller;
    const requestID = ++searchRequestIDRef.current;
    setLoading(true);
    setError(null);
    try {
      const result = await api.searchCatalogFoods(normalized, { exact: true, signal: controller.signal });
      if (requestID !== searchRequestIDRef.current) return;
      setSearchFoods(result.foods);
      setCorrectedQuery("");
      setSearchPage(result.page);
      setSearchHasMore(result.hasMore);
      setExternalSearchUnavailable(Boolean(result.externalUnavailable));
      setVisibleFoodCount(20);
    } catch (cause) {
      if (controller.signal.aborted || requestID !== searchRequestIDRef.current) return;
      setError(cause instanceof Error ? cause.message : "Не удалось выполнить поиск");
    } finally {
      if (requestID === searchRequestIDRef.current) setLoading(false);
    }
  }

  async function showMoreFoods() {
    if (loadingMore) return;
    if (visibleFoodCount < mergedFoods.length) {
      setVisibleFoodCount((count) => count + 20);
      return;
    }
    if (!searchHasMore) {
      return;
    }
    const normalized = query.trim();
    if (normalized.length < 3) return;
    const controller = new AbortController();
    searchAbortRef.current?.abort();
    searchAbortRef.current = controller;
    const requestID = ++searchRequestIDRef.current;
    setLoadingMore(true);
    setError(null);
    try {
      const result = await api.searchCatalogFoods(normalized, { page: searchPage + 1, signal: controller.signal });
      if (query.trim() !== normalized || requestID !== searchRequestIDRef.current) return;
      setSearchFoods((current) => [...current, ...result.foods]);
      setSearchPage(result.page);
      setSearchHasMore(result.hasMore);
      setExternalSearchUnavailable(Boolean(result.externalUnavailable));
      setVisibleFoodCount((count) => count + 20);
    } catch (cause) {
      if (controller.signal.aborted || requestID !== searchRequestIDRef.current) return;
      setError(cause instanceof Error ? cause.message : "Не удалось загрузить следующую страницу");
    } finally {
      if (requestID === searchRequestIDRef.current) setLoadingMore(false);
    }
  }

  function prepareFood(food: FatSecretFood, preferredServingID = "", preferredUnits?: number, recentFallback = false) {
    const servings = food.servings ?? [];
    if (servings.length === 0) {
      setError("FatSecret не вернул доступные порции для этого продукта.");
      return;
    }
    const preferredServing = servings.find((item) => item.id === preferredServingID);
    const measuredPortion = servings.find((item) => item.numberOfUnits === 1
      && Boolean(item.metricAmount && item.metricAmount > 0)
      && (item.metricUnit === "g" || item.metricUnit === "ml"));
    const portionServing = measuredPortion ?? servings.find((item) => item.id === "portion");
    const serving: FatSecretFoodServing = preferredServing ?? portionServing ?? servings[0];
    setSelectedFood(food);
    setSelectedFromRecentFallback(recentFallback);
    setServingID(serving.id);
    setUnits(String(preferredServing && preferredUnits && preferredUnits > 0 ? preferredUnits : serving.numberOfUnits || 1));
    setError(null);
  }

  async function chooseFood(food: FatSecretFood) {
    setLoading(true);
    setError(null);
    try {
      let details = food;
      if (food.provider === "open_food_facts" || !food.servings || food.servings.length === 0) {
        const loaded = await api.localFood(food.id);
        details = {
          ...loaded,
          name: food.name || loaded.name,
          brandName: food.brandName || loaded.brandName,
          description: food.description || loaded.description,
          type: food.type || loaded.type,
        };
      }
      prepareFood(details);
    } catch (cause) {
      // The recently-eaten API already returns the two identifiers required by
      // food_entry.create. Keep that useful path available when the Consumer
      // Key cannot read the regional catalogue entry itself.
      if (food.servingId) {
        const previousUnits = food.units && food.units > 0 ? food.units : 1;
        prepareFood({
          ...food,
          servings: [{
            id: food.servingId,
            description: "Последняя выбранная порция",
            numberOfUnits: previousUnits,
            calories: 0,
            carbohydrate: 0,
            protein: 0,
            fat: 0,
          }],
        }, food.servingId, previousUnits, true);
      } else {
        setError(cause instanceof Error ? cause.message : "Не удалось загрузить порции продукта");
      }
    } finally {
      setLoading(false);
    }
  }

  async function lookupBarcode(raw: string) {
    const code = normalizeBarcodeValue(raw);
    if (!code) {
      setError("Не удалось прочитать штрихкод. Поддерживаются EAN, UPC и DataBar.");
      return;
    }
    setBarcode(code);
    setLoading(true);
    setError(null);
    try {
      const food = await api.localFoodBarcode(code);
      prepareFood(food);
    } catch {
      setError("Продукт не найден в базе. Добавь его ниже!");
      setCreateMode(true);
    } finally {
      setLoading(false);
    }
  }

  async function scanBarcodePhoto(file: File | null) {
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      setError("Выберите фотографию штрихкода.");
      return;
    }
    if (file.size > 12 * 1024 * 1024) {
      setError("Фотография слишком большая. Максимальный размер — 12 МБ.");
      return;
    }
    setLoading(true);
    setError(null);
    const source = URL.createObjectURL(file);
    try {
      const image = await loadBarcodeImage(source);
      const code = await decodeBarcodeFromImage(image);
      await lookupBarcode(code);
    } catch {
      setError("Не удалось распознать штрихкод на снимке. Наведите камеру прямо на полосы и убедитесь, что они занимают большую часть кадра.");
    } finally {
      URL.revokeObjectURL(source);
      setLoading(false);
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  }

  function openBarcodeScanner() {
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
    setError(null);
    setBarcodeScannerOpen(true);
  }

  function openBarcodePhotoFallback() {
    setBarcodeScannerOpen(false);
    fileInputRef.current?.click();
  }

  function handleScannedBarcode(code: string) {
    setBarcodeScannerOpen(false);
    void lookupBarcode(code);
  }

  function handleScannedNutrition(nutrition: ScannedNutrition) {
    setNutritionScannerOpen(false);
    if (nutrition.calories !== undefined) setDraftCalories(String(nutrition.calories).replace(".", ","));
    if (nutrition.protein !== undefined) setDraftProtein(String(nutrition.protein).replace(".", ","));
    if (nutrition.fat !== undefined) setDraftFat(String(nutrition.fat).replace(".", ","));
    if (nutrition.carbohydrate !== undefined) setDraftCarbs(String(nutrition.carbohydrate).replace(".", ","));
    const missing = [
      nutrition.calories === undefined ? "ккал" : "",
      nutrition.protein === undefined ? "белки" : "",
      nutrition.fat === undefined ? "жиры" : "",
      nutrition.carbohydrate === undefined ? "углеводы" : "",
    ].filter(Boolean);
    setError(missing.length > 0 ? `Заполнено всё, что удалось прочитать. Проверьте: ${missing.join(", ")}.` : null);
  }

  async function saveEntry() {
    if (!selectedFood || !selectedServing || !unitsValid || saving) return;
    setSaving(true);
    setError(null);
    const rawName = selectedFood.name.trim() || entry?.name || "Продукт";
    const name = Array.from(rawName).slice(0, 160).join("");
    try {
      const nutrition = entry
        ? await api.updateLocalNutritionEntry(entry.id, { foodId: selectedFood.id, name, servingId: selectedServing.id, numberOfUnits: parsedUnits, meal, date })
        : await api.createLocalNutritionEntry({ foodId: selectedFood.id, name, servingId: selectedServing.id, numberOfUnits: parsedUnits, meal, date });
      onSaved(nutrition);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось сохранить продукт");
    } finally {
      setSaving(false);
    }
  }

  async function createFood() {
    const values = [draftCalories, draftProtein, draftFat, draftCarbs].map((value) => value.trim() === "" ? Number.NaN : Number(value.replace(",", ".")));
    if (!draftName.trim() || values.some((value) => !Number.isFinite(value) || value < 0) || values[0] > 1000 || values.slice(1).some((value) => value > 100) || !portionAmountIsValid) {
      setError(`Проверьте название, КБЖУ на 100 ${nutritionUnitLabel} и размер порции. Порция должна быть от 0,1 до 10 000 ${nutritionUnitLabel}.`);
      return;
    }
    const selectedLocalFood = selectedFood as (FatSecretFood & Partial<LocalFood>) | null;
    const foodBarcode = correctionMode ? selectedLocalFood?.barcode : barcode;
    setLoading(true); setError(null);
    try {
      const food = await api.createLocalFood({ name: draftName.trim(), brandName: draftBrand.trim(), caloriesPer100g: values[0], proteinPer100g: values[1], fatPer100g: values[2], carbohydratePer100g: values[3], nutritionUnit: draftNutritionUnit, portionAmount: parsedPortionAmount, barcode: foodBarcode || undefined });
      const preferredServingID = correctionMode ? servingID : parsedPortionAmount > 0 ? "portion" : draftNutritionUnit;
      const preferredUnits = correctionMode && unitsValid ? parsedUnits : undefined;
      setCreateMode(false); setCorrectionMode(false); prepareFood(food, preferredServingID, preferredUnits);
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Не удалось создать продукт"); } finally { setLoading(false); }
  }

  async function linkFoodCandidate(food: LocalFood) {
    if (!barcode || candidateLinking) return;
    setCandidateLinking(food.id);
    setError(null);
    try {
      const linked = await api.linkLocalFoodBarcode(barcode, food.id);
      setCreateMode(false);
      setFoodCandidates([]);
      prepareFood(linked);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось связать штрихкод с продуктом");
    } finally {
      setCandidateLinking(null);
    }
  }

  function openCreateFoodForm() {
    setDraftName(query.trim());
    setDraftBrand("");
    setDraftCalories("");
    setDraftProtein("");
    setDraftFat("");
    setDraftCarbs("");
    setDraftNutritionUnit("g");
    setDraftPortionAmount("");
    setCreateMode(true);
    setError(null);
  }

  function startFoodCorrection() {
    if (!selectedFood) return;
    const food = selectedFood as FatSecretFood & Partial<LocalFood>;
    const referenceServing = selectedFood.servings?.find((serving) => serving.id === "g" || serving.id === "ml") ?? selectedFood.servings?.[0];
    const portionServing = selectedFood.servings?.find((serving) => serving.id === "portion");
    setDraftName(selectedFood.name);
    setDraftBrand(selectedFood.brandName ?? "");
    setDraftCalories(String(food.caloriesPer100g ?? referenceServing?.calories ?? ""));
    setDraftProtein(String(food.proteinPer100g ?? referenceServing?.protein ?? ""));
    setDraftFat(String(food.fatPer100g ?? referenceServing?.fat ?? ""));
    setDraftCarbs(String(food.carbohydratePer100g ?? referenceServing?.carbohydrate ?? ""));
    setDraftNutritionUnit(referenceServing?.metricUnit === "ml" ? "ml" : "g");
    setDraftPortionAmount(portionServing?.metricAmount ? String(portionServing.metricAmount) : "");
    setCorrectionMode(true);
    setError(null);
  }

  async function deleteSelectedFood() {
    if (!selectedCatalogFood || selectedCatalogFood.provider !== "local" || deletingFood) return;
    if (!window.confirm(`Удалить «${selectedCatalogFood.name}» из ваших продуктов? Записи в дневнике останутся без изменений.`)) return;
    const deletedID = selectedCatalogFood.id;
    setDeletingFood(true);
    setError(null);
    try {
      await api.deleteLocalFood(deletedID);
      setRecentFoods((current) => current.filter((food) => food.id !== deletedID));
      setSearchFoods((current) => current.filter((food) => food.id !== deletedID));
      setSelectedFood(null);
      setSelectedFromRecentFallback(false);
      if (entry) {
        onClose();
        return;
      }
      const normalized = query.trim();
      if (pickerTab === "search" && normalized.length >= 3) {
        try {
          const result = await api.searchCatalogFoods(normalized);
          setSearchFoods(result.foods);
          setCorrectedQuery(result.correctedQuery ?? "");
          setSearchPage(result.page);
          setSearchHasMore(result.hasMore);
          setExternalSearchUnavailable(Boolean(result.externalUnavailable));
          setVisibleFoodCount(20);
        } catch {
          setError("Продукт удалён, но результаты поиска не удалось обновить");
        }
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось удалить продукт");
    } finally {
      setDeletingFood(false);
    }
  }

  const foodDraftEditor = (title: string, submitLabel: string, onCancel?: () => void) => (
    <section className="foodPortionEditor foodDataEditor">
      <h3>{title}</h3>
      {correctionMode && <p className="foodCorrectionNote">{(selectedFood as (FatSecretFood & Partial<LocalFood>) | null)?.barcode
        ? "Исправление будет видно только вам и станет использоваться при следующих поисках этого штрихкода."
        : "Исправление будет видно только вам, а личная версия продукта появится в обычном поиске."}</p>}
      <label className="foodPickerField"><span>Название</span><input value={draftName} maxLength={300} onChange={(event) => setDraftName(event.target.value)} /></label>
      <label className="foodPickerField"><span>Бренд</span><input value={draftBrand} maxLength={300} onChange={(event) => setDraftBrand(event.target.value)} /></label>
      <label className="foodPickerField foodNutritionFormat"><span>Формат записи</span><select value={draftNutritionUnit} onChange={(event) => setDraftNutritionUnit(event.target.value as "g" | "ml")}><option value="g">100 г</option><option value="ml">100 мл</option></select></label>
      <label className="foodPickerField foodPortionDefinition"><span>Одна порция <small>необязательно</small></span><span className="foodPortionDefinitionControl"><b>1 порция =</b><input value={draftPortionAmount} inputMode="decimal" placeholder="—" aria-invalid={!portionAmountIsValid} onChange={(event) => setDraftPortionAmount(event.target.value)} /><em>{nutritionUnitLabel}</em></span></label>
      <div className="foodPortionControls">
        <label className="foodPickerField"><span>Ккал / 100 {nutritionUnitLabel}</span><input inputMode="decimal" value={draftCalories} onChange={(event) => setDraftCalories(event.target.value)} /></label>
        <label className="foodPickerField"><span>Белки</span><input inputMode="decimal" value={draftProtein} onChange={(event) => setDraftProtein(event.target.value)} /></label>
      </div>
      <div className="foodPortionControls">
        <label className="foodPickerField"><span>Жиры</span><input inputMode="decimal" value={draftFat} onChange={(event) => setDraftFat(event.target.value)} /></label>
        <label className="foodPickerField"><span>Углеводы</span><input inputMode="decimal" value={draftCarbs} onChange={(event) => setDraftCarbs(event.target.value)} /></label>
      </div>
      {!correctionMode && <button type="button" className="nutritionScanButton" onClick={() => {
        if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
        setError(null);
        setNutritionScannerOpen(true);
      }}><span aria-hidden="true">▣</span><strong>Сканировать КБЖУ с этикетки</strong><small>Заполним только калории, белки, жиры и углеводы</small></button>}
      {!foodCandidatesLoading && foodCandidates.length > 0 && (
        <section className="foodCandidateMatches" aria-label="Похожие продукты">
          <header><strong>Возможно, продукт уже есть</strong><small>Сравнили КБЖУ на 100 {nutritionUnitLabel}. Связывание не изменит существующую карточку.</small></header>
          {foodCandidates.map((food) => (
            <article className="foodCandidateRow" key={food.id}>
              <span><strong>{food.name}</strong><small>{food.brandName || "Без марки"}</small></span>
              <span className="foodCandidateMacros"><b>{formatNutrition(food.caloriesPer100g, 0)} ккал</b><small>Б {formatNutrition(food.proteinPer100g)} · Ж {formatNutrition(food.fatPer100g)} · У {formatNutrition(food.carbohydratePer100g)}</small></span>
              <button type="button" disabled={candidateLinking !== null} onClick={() => void linkFoodCandidate(food)}>{candidateLinking === food.id ? "Связываем…" : "Связать"}</button>
            </article>
          ))}
        </section>
      )}
      {!foodCandidatesLoading && foodCandidatesError && <div className="formError" role="alert">{foodCandidatesError}</div>}
      {visibleError && <div className="formError" role="alert">{visibleError}</div>}
      <div className={`modalActions foodCorrectionActions${onCancel ? "" : " foodCorrectionActionsSingle"}`}>
        {onCancel && <button type="button" className="secondaryButton" disabled={loading} onClick={onCancel}>Отмена</button>}
        <button type="button" className="primaryButton" disabled={loading} onClick={() => void createFood()}>{loading ? "Сохранение…" : submitLabel}</button>
      </div>
    </section>
  );

  return (
    <Modal title={selectedFood ? "Продукт" : entry ? "Изменить продукт" : `Добавить в ${mealMeta.label}`} wide className="foodPickerModal" onClose={onClose}>
      <div className="foodPicker">
        {selectedFood ? (
          <section className="foodPortionEditor">
            {correctionMode ? foodDraftEditor("Исправить продукт", "Сохранить исправление", () => { setCorrectionMode(false); setError(null); }) : <>
              <button type="button" className="foodPickerBack" onClick={() => { if (!entry) { setSelectedFood(null); setSelectedFromRecentFallback(false); } else onClose(); }}><span aria-hidden="true">‹</span>К результатам</button>
              <div className="foodSelectedTitle">
                <h2>{selectedFood.name}</h2>
                {selectedFood.brandName && <p>{selectedFood.brandName}</p>}
              </div>
              <div className="foodDiaryAddTitle"><strong>Добавить в {mealMeta.label.toLocaleLowerCase("ru-RU")}</strong><span>{formatDate(date)}</span></div>
              {portionVariants.length > 1 && <section className="foodServingVariants" aria-labelledby="food-serving-variants-title">
                <span id="food-serving-variants-title">Размер порции</span>
                <div>{portionVariants.map((serving) => <button
                  type="button"
                  key={serving.id}
                  className={serving.id === servingID ? "isSelected" : ""}
                  aria-pressed={serving.id === servingID}
                  onClick={() => { setServingID(serving.id); setUnits(String(serving.numberOfUnits || 1)); }}
                >{serving.description}</button>)}</div>
              </section>}
              <div className="foodPortionControls">
                <label className="foodPickerField foodAmountField"><span>Количество</span><input value={units} inputMode="decimal" onFocus={(event) => event.currentTarget.select()} onClick={(event) => event.currentTarget.select()} onChange={(event) => setUnits(event.target.value)} aria-invalid={!unitsValid} /></label>
                <label className="foodPickerField foodServingField"><span>Порция или единица</span><select value={servingID} onChange={(event) => {
                  const next = selectedFood.servings?.find((serving) => serving.id === event.target.value);
                  setServingID(event.target.value);
                  if (next) setUnits(String(next.numberOfUnits || 1));
                }}>{selectedFood.servings?.map((serving) => <option key={serving.id} value={serving.id}>{serving.description}</option>)}</select></label>
              </div>
              <button type="button" className="primaryButton foodDiarySave" disabled={!unitsValid || saving || loading} onClick={() => void saveEntry()}>{saving ? "Сохранение…" : entry ? "Сохранить изменения" : "Добавить в дневник"}</button>
              {selectedFromRecentFallback && <p className="foodRecentFallbackNote">FatSecret не отдал полную карточку продукта. Последнюю порцию всё равно можно добавить; итоговые КБЖУ обновятся из дневника после сохранения.</p>}
              {selectedServing && !selectedFromRecentFallback && (
                <div className="foodPortionMacros">
                  <div><span>Калории</span><strong>{formatNutrition(selectedServing.calories * multiplier, 0)}</strong><small>ккал</small></div>
                  <div><span>Жиры</span><strong>{formatNutrition(selectedServing.fat * multiplier)}</strong><small>г</small></div>
                  <div><span>Углеводы</span><strong>{formatNutrition(selectedServing.carbohydrate * multiplier)}</strong><small>г</small></div>
                  <div><span>Белки</span><strong>{formatNutrition(selectedServing.protein * multiplier)}</strong><small>г</small></div>
                </div>
              )}
              <button type="button" className="foodCorrectionButton" onClick={startFoodCorrection}>Название или КБЖУ неверны? Исправить</button>
              {canDeleteSelectedFood && <button type="button" className="foodDeleteButton" disabled={deletingFood || saving || loading} onClick={() => void deleteSelectedFood()}>{deletingFood ? "Удаляем…" : "Удалить из моих продуктов"}</button>}
            </>}
            {!correctionMode && visibleError && <div className="formError" role="alert">{visibleError}</div>}
          </section>
        ) : (
          <>
            <div className="foodPickerTabs" role="tablist" aria-label="Выбор еды">
              <button type="button" role="tab" aria-selected={pickerTab === "recent"} className={pickerTab === "recent" ? "isActive" : ""} onClick={() => { setPickerTab("recent"); setError(null); }}>Последняя еда</button>
              <button type="button" role="tab" aria-selected={pickerTab === "search"} className={pickerTab === "search" ? "isActive" : ""} onClick={() => { setPickerTab("search"); setError(null); }}>Поиск</button>
            </div>
            {pickerTab === "recent" ? (
              <div className="foodPickerTabPanel" role="tabpanel">
                <div className="foodRecentScopeTabs" role="tablist" aria-label="Период последней еды">
                  <button type="button" role="tab" aria-selected={recentScope === "meal"} className={recentScope === "meal" ? "isActive" : ""} onClick={() => setRecentScope("meal")}>{meal === "other" ? "Перекус" : mealMeta.label}</button>
                  <button type="button" role="tab" aria-selected={recentScope === "all"} className={recentScope === "all" ? "isActive" : ""} onClick={() => setRecentScope("all")}>Все время</button>
                </div>
                <div className="foodRecentScopePanel" role="tabpanel">
                  {recentLoading && <div className="nutritionSync" role="status">Загружаем последнюю еду…</div>}
                  {visibleError && <div className="formError" role="alert">{visibleError}</div>}
                  {!recentLoading && recentFoods.length > 0 && <section className="foodResults foodRecentResults"><h3>Недавно добавляли</h3>{recentFoods.map((food) => <FoodResultRow key={`${food.id}:${food.servingId}`} food={food} onSelect={chooseFood} />)}</section>}
                  {!recentLoading && !visibleError && recentFoods.length === 0 && <div className="foodRecentEmpty"><strong>Здесь пока пусто</strong><p>Еда, которую вы добавите через поиск, появится в этой вкладке.</p><button type="button" onClick={() => setPickerTab("search")}>Перейти к поиску</button></div>}
                </div>
              </div>
            ) : (
              <div className="foodPickerTabPanel" role="tabpanel">
                <div className="foodSearchToolbar">
                  <div className="foodSearchField">
                    <span aria-hidden="true">⌕</span>
                    <label className="srOnly" htmlFor="food-catalog-search">Поиск продуктов</label>
                    <input
                      ref={searchInputRef}
                      id="food-catalog-search"
                      value={query}
                      autoFocus
                      autoComplete="off"
                      placeholder="Название, бренд или штрихкод"
                      onCompositionStart={() => setQueryComposing(true)}
                      onCompositionEnd={() => setQueryComposing(false)}
                      onChange={(event) => setQuery(event.target.value)}
                    />
                    {query && <button type="button" className="foodSearchClear" aria-label="Очистить поиск" onClick={() => {
                      searchRequestIDRef.current += 1;
                      searchAbortRef.current?.abort();
                      setQuery("");
                      searchInputRef.current?.focus();
                    }}>×</button>}
                  </div>
                  <button type="button" className="barcodeScanIconButton" aria-label="Сканировать штрихкод" title="Сканировать штрихкод" onClick={openBarcodeScanner}>
                    <img src="/barcode-scanner.png" alt="" />
                  </button>
                </div>
                {correctedQuery && <div className="foodSearchCorrection"><span>Показываем результаты для «{correctedQuery}»</span><button type="button" onClick={() => void searchExactlyAsTyped()}>Искать как введено</button></div>}
                {externalSearchUnavailable && <div className="foodSearchNotice" role="status">Показаны результаты из каталога. Внешний поиск временно недоступен.</div>}
                <input ref={fileInputRef} className="foodBarcodeFile" type="file" accept="image/*" capture="environment" onChange={(event) => void scanBarcodePhoto(event.target.files?.[0] ?? null)} />
                {loading && <div className="nutritionSync" role="status">Ищем продукты…</div>}
                {!createMode && visibleError && <div className="formError" role="alert">{visibleError}</div>}
                {query.trim().length >= 3 && (
                  <section className="foodResults">
                    <h3>Результаты поиска</h3>
                    {visibleFoods.length > 0 ? visibleFoods.map((food) => <FoodResultRow key={food.id} food={food} onSelect={chooseFood} />) : !loading && !visibleError && <p>Ничего не найдено</p>}
                    {!loading && !createMode && visibleFoods.length > 0 && searchPage === 0 && visibleFoodCount <= 20 && <button type="button" className="foodAddProductRow" onClick={openCreateFoodForm}><span aria-hidden="true">＋</span><strong>Добавить продукт</strong></button>}
                    {!loading && !createMode && visibleFoods.length === 0 && !visibleError && <button type="button" className="foodAddProductRow" onClick={openCreateFoodForm}><span aria-hidden="true">＋</span><strong>Добавить продукт</strong></button>}
                    {canShowMoreFoods && <button type="button" className="foodResultsMore" disabled={loadingMore} onClick={() => void showMoreFoods()}>{loadingMore ? "Загружаем…" : "Показать ещё"}</button>}
                  </section>
                )}
                {createMode && foodDraftEditor("Новый продукт", "Создать и выбрать")}
              </div>
            )}
            <div className="modalActions foodPickerActions"><button type="button" className="secondaryButton" onClick={onClose}>Отмена</button></div>
          </>
        )}
        <a className="fatSecretAttribution fatSecretAttributionBottom" href="https://platform.fatsecret.com" target="_blank" rel="noreferrer">Powered by fatsecret Platform API</a>
      </div>
      {barcodeScannerOpen && <Suspense fallback={null}><BarcodeScannerSheet onDetected={handleScannedBarcode} onClose={() => setBarcodeScannerOpen(false)} onFallback={openBarcodePhotoFallback} /></Suspense>}
      {nutritionScannerOpen && <Suspense fallback={null}><NutritionScannerSheet onDetected={handleScannedNutrition} onClose={() => setNutritionScannerOpen(false)} /></Suspense>}
    </Modal>
  );
}

function FoodResultRow({ food, onSelect }: { food: FatSecretFood; onSelect: (food: FatSecretFood) => void | Promise<void> }) {
  const title = food.brandName ? `${food.name} · ${food.brandName}` : food.name;
  const details = food.description || (food.type === "Brand" ? "Брендовый продукт" : "Обычный продукт");
  const catalogFood = food as FatSecretFood & Partial<LocalFood>;
  const hasNutrition = typeof catalogFood.caloriesPer100g === "number";
  return (
    <button type="button" className="foodResultRow" onClick={() => void onSelect(food)}>
      <span className="foodResultText">
        <strong>{title}</strong>
        <small>{food.provider === "local" ? `Ваш продукт · ${details}` : details}</small>
        {hasNutrition && <span className="foodResultNutrition">
          {formatNutrition(catalogFood.caloriesPer100g ?? 0, 0)} ккал · Б {formatNutrition(catalogFood.proteinPer100g ?? 0)} · Ж {formatNutrition(catalogFood.fatPer100g ?? 0)} · У {formatNutrition(catalogFood.carbohydratePer100g ?? 0)}
        </span>}
      </span>
      <b aria-hidden="true">›</b>
    </button>
  );
}

function mergeFatSecretFoodResults(apiFoods: FatSecretFood[], recentFoods: FatSecretFood[], ...queries: Array<string | undefined>) {
  const normalizedQueries = queries
    .map((query) => (query ?? "").trim().toLocaleLowerCase("ru-RU").replace(/ё/g, "е"))
    .filter(Boolean);
  const matchesQuery = (food: FatSecretFood) => {
    if (normalizedQueries.length === 0) return false;
    const searchable = `${food.name} ${food.brandName ?? ""}`.toLocaleLowerCase("ru-RU").replace(/ё/g, "е");
    return normalizedQueries.some((query) => query.split(/\s+/).every((part) => searchable.includes(part)));
  };
  const seenRecent = new Set<string>();
  const recentMatches = recentFoods.filter((food) => {
    if (!matchesQuery(food) || seenRecent.has(food.id)) return false;
    seenRecent.add(food.id);
    return true;
  });
  // Server ranking is authoritative. Recent entries only provide saved serving
  // details or fill gaps; they must not jump ahead of more relevant matches.
  const recentByID = new Map(recentMatches.map((food) => [food.id, food]));
  const seen = new Set<string>();
  const merged: FatSecretFood[] = [];
  for (const food of apiFoods) {
    if (seen.has(food.id)) continue;
    seen.add(food.id);
    const recent = recentByID.get(food.id);
    merged.push(recent ? {
      ...food,
      servingId: recent.servingId || food.servingId,
      units: recent.units || food.units,
    } : food);
  }
  for (const recent of recentMatches) {
    if (!seen.has(recent.id)) merged.push(recent);
  }
  return merged;
}

function TrackerStatistics({
  currentDate,
  selected,
  state,
  nutrition,
  fatSecretConnected,
  calorieMode,
  calorieGoal,
  github,
  githubPeriod,
  onGitHubPeriod,
}: {
  currentDate: string;
  selected: TrackerSelectionID[];
  state: TrackerState;
  nutrition: FatSecretNutrition | null;
  fatSecretConnected: boolean;
  calorieMode: CalorieAccountingMode | null;
  calorieGoal: number;
  github: GitHubTrackerState | null;
  githubPeriod: GitHubPeriod;
  onGitHubPeriod: (period: GitHubPeriod) => void;
}) {
  const [period, setPeriod] = useState<7 | 30>(7);
  const [selectedCalorieDate, setSelectedCalorieDate] = useState(currentDate);
  const [calorieHistory, setCalorieHistory] = useState<Record<string, number | null>>({});
  const [calorieHistoryLoading, setCalorieHistoryLoading] = useState(false);
  const loadedCalorieDates = useRef(new Map<string, number | null>());
  const dates = useMemo(
    () => Array.from({ length: period }, (_, index) => addDaysToDateKey(currentDate, index - period + 1)),
    [currentDate, period],
  );
  const firstDate = dates[0];
  const waterByDate = new Map(state.waterHistory.map((entry) => [entry.date, entry]));
  const weightByDate = new Map(state.weightHistory.map((entry) => [entry.date, entry.weightKg]));
  const waterValues = dates.map((date) => waterByDate.get(date)?.glasses ?? 0);
  const waterGoals = dates.map((date) => waterByDate.get(date)?.goalGlasses ?? state.waterGoal);
  const waterAverage = waterValues.reduce((sum, value) => sum + value, 0) / Math.max(1, waterValues.length);
  const waterGoalDays = waterValues.filter((value, index) => value >= waterGoals[index]).length;
  const weightValues = dates.map((date) => weightByDate.get(date) ?? null);
  const measuredWeights = weightValues.filter((value): value is number => value !== null);
  const weightDelta = measuredWeights.length > 1 ? measuredWeights[measuredWeights.length - 1] - measuredWeights[0] : null;
  const calorieValue = nutrition ? Math.round(nutrition.calories) : null;
  const calorieValues = dates.map((date) => date === currentDate && calorieValue !== null
    ? calorieValue
    : calorieHistory[date] ?? null);
  const measuredCalories = calorieValues.filter((value): value is number => value !== null);
  const calorieAverage = measuredCalories.reduce((sum, value) => sum + value, 0) / Math.max(1, measuredCalories.length);
  const calorieGoalDays = calorieValues.filter((value) => value !== null && value <= calorieGoal).length;
  const selectedCalorieIndex = dates.indexOf(selectedCalorieDate);
  const selectedCalorieValue = selectedCalorieIndex >= 0 ? calorieValues[selectedCalorieIndex] : null;
  const selectedCaloriePending = (calorieMode === "internal" || fatSecretConnected)
    && calorieHistoryLoading
    && selectedCalorieDate !== currentDate
    && !loadedCalorieDates.current.has(selectedCalorieDate);
  const visibleTrackers = selected.length > 0
    ? selected
    : (["calories", "water", "weight"] as TrackerSelectionID[]);
  const caloriesVisible = visibleTrackers.includes("calories");
  const calorieDataAvailable = calorieMode === "internal" || fatSecretConnected;

  useEffect(() => {
    if (!dates.includes(selectedCalorieDate)) setSelectedCalorieDate(currentDate);
  }, [currentDate, dates, selectedCalorieDate]);

  useEffect(() => {
    let active = true;
    if (calorieValue !== null) {
      loadedCalorieDates.current.set(currentDate, calorieValue);
      setCalorieHistory((items) => items[currentDate] === calorieValue ? items : { ...items, [currentDate]: calorieValue });
    }
    if (!calorieDataAvailable || !caloriesVisible) return () => { active = false; };
    const missing = dates.filter((date) => date !== currentDate && !loadedCalorieDates.current.has(date));
    if (missing.length === 0) return () => { active = false; };
    setCalorieHistoryLoading(true);
    let cursor = 0;
    const results: Record<string, number | null> = {};
    async function worker() {
      while (active && cursor < missing.length) {
        const date = missing[cursor++];
        try {
          const day = calorieMode === "fatsecret" ? await api.fatSecretNutrition(date) : await api.localNutrition(date);
          results[date] = Math.round(day.calories);
        } catch {
          results[date] = null;
        }
        if (!active) return;
        loadedCalorieDates.current.set(date, results[date]);
        setCalorieHistory((items) => ({ ...items, [date]: results[date] }));
      }
    }
    void Promise.all(Array.from({ length: Math.min(3, missing.length) }, () => worker())).then(() => {
      if (!active) return;
      setCalorieHistory((items) => ({ ...items, ...results }));
      setCalorieHistoryLoading(false);
    });
    return () => { active = false; };
  }, [calorieDataAvailable, calorieMode, calorieValue, currentDate, dates, caloriesVisible]);

  return (
    <div className="trackerStatistics">
      <header className="trackerStatisticsHeader">
        <div>
          <strong>Трекеры</strong>
          <span>{formatStatisticsRange(firstDate, currentDate)}</span>
        </div>
        <div className="trackerStatisticsPeriod" role="group" aria-label="Период статистики">
          <button type="button" className={period === 7 ? "active" : ""} aria-pressed={period === 7} onClick={() => setPeriod(7)}>7 дней</button>
          <button type="button" className={period === 30 ? "active" : ""} aria-pressed={period === 30} onClick={() => setPeriod(30)}>30 дней</button>
        </div>
      </header>

      <div className="trackerStatisticsGrid">
        {visibleTrackers.includes("calories") && (
          <article className="trackerStatisticsCard trackerStatisticsWide trackerStatisticsCalories">
            <header><span className="trackerStatisticsIcon"><FlameIcon /></span><div><strong>Калории</strong><small>{measuredCalories.length > 0 ? `Среднее: ${formatNutrition(calorieAverage, 0)} ккал в день` : calorieMode === "internal" ? "Данные дневника по дням" : "Данные FatSecret по дням"}</small></div>{calorieValue !== null && <b>{formatNutrition(calorieValue, 0)} ккал</b>}</header>
            {calorieDataAvailable
              ? <>
                <DailyBarChart
                  dates={dates}
                  values={calorieValues}
                  goals={dates.map(() => calorieGoal)}
                  period={period}
                  suffix="ккал"
                  goalDirection="maximum"
                  ariaLabel="Столбчатый график калорий по дням"
                  currentDate={currentDate}
                  selectedDate={selectedCalorieDate}
                  onSelectDate={setSelectedCalorieDate}
                />
                <div className="calorieSelectedDay" aria-live="polite">
                  <span>{selectedCalorieDate === currentDate ? "Сегодня" : formatStatisticsDay(selectedCalorieDate)}</span>
                  <strong>{selectedCaloriePending ? "Загрузка…" : selectedCalorieValue === null ? "Нет данных" : `${formatNutrition(selectedCalorieValue, 0)} ккал`}</strong>
                  <small>Дневная норма · {formatNutrition(calorieGoal, 0)} ккал</small>
                </div>
              </>
              : <div className="dailyChartEmpty">Подключите FatSecret, чтобы увидеть статистику калорий</div>}
            <p>{calorieHistoryLoading ? "Загружаем дневник за выбранный период…" : measuredCalories.length > 0 ? `В пределах дневной нормы: ${calorieGoalDays} ${pluralDays(calorieGoalDays)}.` : "Нет данных за выбранный период."}</p>
          </article>
        )}

        {visibleTrackers.includes("water") && (
          <article className="trackerStatisticsCard trackerStatisticsWide">
            <header><span className="trackerStatisticsIcon"><WaterIcon /></span><div><strong>Вода</strong><small>В среднем {formatTrackerNumber(waterAverage)} стакана в день</small></div><b>{waterGoalDays}/{period}</b></header>
            <DailyBarChart dates={dates} values={waterValues} goals={waterGoals} period={period} suffix="стаканов" />
            <p>Дневная цель выполнена: {waterGoalDays} {pluralDays(waterGoalDays)}.</p>
          </article>
        )}

        {visibleTrackers.includes("weight") && (
          <article className="trackerStatisticsCard trackerStatisticsWide">
            <header><span className="trackerStatisticsIcon"><DumbbellIcon /></span><div><strong>Вес</strong><small>{measuredWeights.length > 0 ? `${measuredWeights.length} измерений` : "Пока нет измерений"}</small></div>{weightDelta !== null && <b>{weightDelta > 0 ? "+" : ""}{formatTrackerWeight(weightDelta)} кг</b>}</header>
            {measuredWeights.length === 1
              ? <div className="weightStatisticsStarting"><div><strong>{formatTrackerWeight(measuredWeights[0])} кг</strong><span>1 измерение</span></div><i><b /></i></div>
              : <DailyLineChart dates={dates} values={weightValues} period={period} suffix="кг" />}
            <p>{measuredWeights.length === 1 ? "Добавьте ещё одно измерение, чтобы увидеть динамику." : weightDelta === null ? "Добавляйте вес по дням — здесь появится динамика." : weightDelta === 0 ? "Вес за период не изменился." : `Изменение за период: ${weightDelta > 0 ? "+" : ""}${formatTrackerWeight(weightDelta)} кг.`}</p>
          </article>
        )}

        {visibleTrackers.includes("github") && (
          <article className="trackerStatisticsCard trackerStatisticsWide trackerStatisticsGitHub">
            <header><span className="trackerStatisticsIcon"><GitHubIcon /></span><div><strong>GitHub активность</strong><small>{github?.configured ? `@${github.username}` : "Username не настроен"}</small></div>{github?.configured && <div className="githubStatisticsValue"><b>{githubContributionCount(github, githubPeriod, currentDate)}</b><span>{githubPeriodMeta(githubPeriod).caption}</span></div>}</header>
            {github?.configured && <GitHubPeriodSelector value={githubPeriod} onChange={onGitHubPeriod} />}
            {github?.configured ? <GitHubHeatmap days={github.days} expanded weeks={githubPeriod === "year" ? GITHUB_TRACKER_WEEKS : GITHUB_TRACKER_CARD_WEEKS} /> : <div className="dailyChartEmpty">Настройте GitHub username на карточке трекера</div>}
            <p>{github?.configured ? "Показана публичная активность из календаря профиля GitHub. Выберите день, текущую неделю или последние 365 дней." : "Укажите username, чтобы загрузить календарь профиля GitHub."}</p>
          </article>
        )}

        {visibleTrackers.map((selection) => {
          const match = /^custom:(\d+)$/.exec(selection);
          if (!match) return null;
          const tracker = state.customTrackers.find((item) => item.id === Number(match[1]));
          if (!tracker) return null;
          const entries = (state.customHistory ?? [])
            .filter((entry) => entry.trackerId === tracker.id)
            .sort((left, right) => left.date.localeCompare(right.date));
          return (
            <CustomTrackerStatisticsCard
              key={selection}
              tracker={tracker}
              entries={entries}
              currentDate={currentDate}
              period={period}
            />
          );
        })}
      </div>
    </div>
  );
}

function CustomTrackerStatisticsCard({ tracker, entries, currentDate, period }: {
  tracker: CustomTracker;
  entries: CustomTrackerEntry[];
  currentDate: string;
  period: 7 | 30;
}) {
  const availableEntries = entries.filter((entry) => entry.date <= currentDate);
  const dates = Array.from({ length: period }, (_, index) => addDaysToDateKey(currentDate, index - period + 1));
  const firstDate = dates[0] ?? currentDate;
  const periodEntries = availableEntries.filter((entry) => entry.date >= firstDate);
  const values = dailyTrackerSeries(dates, availableEntries, currentDate, tracker.currentValue);
  const progress = Math.round(Math.min(100, tracker.currentValue / tracker.targetValue * 100));
  const periodTotal = periodEntries.reduce((sum, entry) => sum + entry.value, 0);
  const completedDays = periodEntries.filter((entry) => entry.value >= entry.targetValue).length;
  const periodLabel = formatStatisticsRange(firstDate, currentDate);

  return (
    <article className="trackerStatisticsCard trackerStatisticsWide customTrackerStatisticsCard">
      <header>
        <span className="trackerStatisticsIcon customTrackerGlyph"><CustomTrackerIcon icon={tracker.icon} /></span>
        <div><strong>{tracker.name}</strong><small>Шаг {formatTrackerNumber(tracker.stepValue)} · {periodLabel}</small></div>
        <b>{progress}%</b>
      </header>
      <div className="customStatisticsSummaryCompact">
        <strong>{formatTrackerNumber(tracker.currentValue)} <small>из {formatTrackerNumber(tracker.targetValue)} сейчас</small></strong>
        <span>{formatTrackerNumber(periodTotal)} выполнений · {periodEntries.length} {pluralDays(periodEntries.length)} с отметками</span>
      </div>
      <div className="customStatisticsProgress" role="progressbar" aria-label="Текущий прогресс" aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress}><span style={{ width: `${progress}%` }} /></div>
      <DailyLineChart dates={dates} values={values} period={period} suffix="" target={tracker.targetValue} />
      <p>{completedDays > 0
        ? `Цель достигалась в ${completedDays} ${pluralDays(completedDays)} за выбранный период.`
        : periodEntries.length > 0
          ? `Отметки были в ${periodEntries.length} ${pluralDays(periodEntries.length)} · цель пока не достигалась.`
          : "За выбранный период изменений не было."}</p>
    </article>
  );
}

function DailyBarChart({ dates, values, goals, period, suffix, goalDirection = "minimum", ariaLabel = "Столбчатый график воды по дням", currentDate, selectedDate, onSelectDate }: {
  dates: string[];
  values: Array<number | null>;
  goals: number[];
  period: 7 | 30;
  suffix: string;
  goalDirection?: "minimum" | "maximum";
  ariaLabel?: string;
  currentDate?: string;
  selectedDate?: string;
  onSelectDate?: (date: string) => void;
}) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const maximum = Math.max(1, ...values.filter((value): value is number => value !== null), ...goals);

  useEffect(() => {
    if (period !== 30 || !viewportRef.current) return;
    viewportRef.current.scrollLeft = viewportRef.current.scrollWidth;
  }, [period]);

  return (
    <div ref={viewportRef} className={`dailyBarViewport dailyBarViewport${period}`} role={onSelectDate ? "group" : "img"} aria-label={ariaLabel}>
      <div className={`dailyBarChart dailyBarChart${period}`}>
        {dates.map((date, index) => {
          const value = values[index];
          const goal = goals[index];
          const showLabel = period === 7 || index === 0 || index === dates.length - 1 || index % 5 === 4;
          const label = value === null ? `${formatStatisticsDay(date)}: нет данных` : `${formatStatisticsDay(date)}: ${formatTrackerNumber(value)} ${suffix}, цель ${formatTrackerNumber(goal)}`;
          const className = `dailyBarColumn ${value === null ? "dailyBarColumnMissing" : ""} ${date === currentDate ? "dailyBarColumnToday" : ""} ${date === selectedDate ? "dailyBarColumnSelected" : ""}`;
          const content = <>
            <div className="dailyBarPlot">
              <i style={{ bottom: `${Math.min(100, goal / maximum * 100)}%` }} />
              {value !== null && <span className={(goalDirection === "minimum" ? value >= goal : value > goal) ? "goalReached" : ""} style={{ height: `${Math.max(value > 0 ? 5 : 0, value / maximum * 100)}%` }} />}
            </div>
            <small>{showLabel ? formatStatisticsBarDay(date, period) : ""}</small>
          </>;
          return onSelectDate
            ? <button type="button" className={className} key={date} title={label} aria-label={label} aria-pressed={date === selectedDate} onClick={() => onSelectDate(date)}>{content}</button>
            : <div className={className} key={date} title={label}>{content}</div>;
        })}
      </div>
    </div>
  );
}

function DailyLineChart({ dates, values, period, suffix, target }: {
  dates: string[];
  values: Array<number | null>;
  period: StatisticsPeriod;
  suffix: string;
  target?: number;
}) {
  const points = values
    .map((value, index) => value === null ? null : { value, index })
    .filter((point): point is { value: number; index: number } => point !== null);
  if (points.length === 0) return <div className="dailyChartEmpty">Нет данных за этот период</div>;
  const allValues = [...points.map((point) => point.value), ...(target === undefined ? [] : [target])];
  const minimum = Math.min(...allValues);
  const maximum = Math.max(...allValues);
  const padding = Math.max((maximum - minimum) * .12, maximum === 0 ? 1 : Math.abs(maximum) * .025, .1);
  const low = Math.max(0, minimum - padding);
  const high = maximum + padding;
  const range = Math.max(.001, high - low);
  const x = (index: number) => 12 + index / Math.max(1, dates.length - 1) * 296;
  const y = (value: number) => 10 + (1 - (value - low) / range) * 76;
  const polyline = points.map((point) => `${x(point.index)},${y(point.value)}`).join(" ");
  const targetY = target === undefined ? null : y(target);
  return (
    <div className="dailyLineChart" role="img" aria-label={`Линейный график по дням${suffix ? `, единица ${suffix}` : ""}`}>
      <svg viewBox="0 0 320 96" preserveAspectRatio="none" aria-hidden="true">
        <path className="dailyLineGrid" d="M12 48H308" />
        {targetY !== null && <path className="dailyLineTarget" d={`M12 ${targetY}H308`} />}
        {points.length > 1 && <polyline points={polyline} />}
        {points.map((point) => <circle key={`${point.index}-${point.value}`} cx={x(point.index)} cy={y(point.value)} r={points.length === 1 ? 4 : 2.8} />)}
      </svg>
      <div className="dailyLineLabels"><span>{formatStatisticsAxisDay(dates[0], period)}</span><strong>{formatTrackerNumber(points[points.length - 1].value)}{suffix ? ` ${suffix}` : ""}</strong><span>{formatStatisticsAxisDay(dates[dates.length - 1], period)}</span></div>
    </div>
  );
}

function dailyTrackerSeries(dates: string[], entries: CustomTrackerEntry[], currentDate: string, currentValue: number) {
  const values = new Map(entries.map((entry) => [entry.date, entry.value]));
  return dates.map((date) => date === currentDate ? currentValue : values.get(date) ?? null);
}

function formatStatisticsDay(value: string) {
  const date = parseDateKey(value);
  if (!date) return value;
  const formatted = date.toLocaleDateString("ru-RU", { weekday: "short", day: "numeric", month: "short" });
  return formatted.replace(".", "");
}

function formatStatisticsAxisDay(value: string, period: StatisticsPeriod) {
  const date = parseDateKey(value);
  if (!date) return value.slice(8);
  return period === 7
    ? date.toLocaleDateString("ru-RU", { weekday: "short" }).replace(".", "")
    : date.toLocaleDateString("ru-RU", { day: "numeric", month: "short" }).replace(".", "");
}

function formatStatisticsBarDay(value: string, period: 7 | 30) {
  if (period === 7) return formatStatisticsAxisDay(value, period);
  return parseDateKey(value)?.getDate().toString() ?? value.slice(8);
}

function formatStatisticsRange(from: string, to: string) {
  const first = parseDateKey(from);
  const last = parseDateKey(to);
  if (!first || !last) return `${from} — ${to}`;
  return `${first.toLocaleDateString("ru-RU", { day: "numeric", month: "short" }).replace(".", "")} — ${last.toLocaleDateString("ru-RU", { day: "numeric", month: "short" }).replace(".", "")}`;
}

function pluralDays(value: number) {
  const tens = value % 100;
  const units = value % 10;
  if (tens >= 11 && tens <= 19) return "дней";
  if (units === 1) return "день";
  if (units >= 2 && units <= 4) return "дня";
  return "дней";
}

function TrackerStepper({
  label,
  value,
  suffix,
  detail,
  decrementDisabled,
  incrementDisabled,
  onDecrease,
  onIncrease,
}: {
  label: string;
  value: number;
  suffix: string;
  detail?: string;
  decrementDisabled: boolean;
  incrementDisabled: boolean;
  onDecrease: () => void;
  onIncrease: () => void;
}) {
  return (
    <section className="trackerStepper">
      <div className="trackerStepperLabel"><span>{label}</span>{detail && <small>{detail}</small>}</div>
      <div className="trackerStepperControls">
        <button type="button" onClick={onDecrease} disabled={decrementDisabled} aria-label={`Уменьшить: ${label}`}>−</button>
        <output><strong>{value}</strong><small>{suffix}</small></output>
        <button type="button" onClick={onIncrease} disabled={incrementDisabled} aria-label={`Увеличить: ${label}`}>＋</button>
      </div>
    </section>
  );
}

function TrackerCardIcon({ card }: { card: TrackerCardID }) {
  if (card === "calories") return <FlameIcon />;
  if (card === "water") return <WaterIcon />;
  if (card === "github") return <GitHubIcon />;
  return <DumbbellIcon />;
}

function GitHubIcon() {
  return (
    <svg className="githubTrackerIcon" viewBox="0 0 16 16" aria-hidden="true">
      <path fill="currentColor" d="M8 0C3.58 0 0 3.64 0 8.13c0 3.59 2.29 6.64 5.47 7.71.4.07.55-.17.55-.38 0-.19-.01-.83-.01-1.5-2.01.44-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.59 1.23.88.72 1.26 1.87.9 2.33.69.07-.53.28-.9.51-1.11-1.78-.21-3.64-.9-3.64-4.01 0-.89.31-1.62.82-2.19-.08-.2-.36-1.04.08-2.16 0 0 .67-.22 2.2.84A7.45 7.45 0 0 1 8 3.8c.68 0 1.36.09 2 .27 1.53-1.06 2.2-.84 2.2-.84.44 1.12.16 1.96.08 2.16.51.57.82 1.3.82 2.19 0 3.12-1.87 3.8-3.65 4.01.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.02 8.02 0 0 0 16 8.13C16 3.64 12.42 0 8 0Z" />
    </svg>
  );
}

function GitHubPeriodSelector({ value, onChange }: { value: GitHubPeriod; onChange: (period: GitHubPeriod) => void }) {
  return (
    <div className="githubPeriodSelector" role="group" aria-label="Период GitHub-статистики">
      {GITHUB_PERIODS.map((period) => (
        <button key={period.value} type="button" className={value === period.value ? "isActive" : ""} aria-pressed={value === period.value} aria-label={`Коммиты за период: ${period.label.toLocaleLowerCase("ru-RU")}`} onClick={() => onChange(period.value)}>
          {period.label}
        </button>
      ))}
    </div>
  );
}

function GitHubHeatmap({ days, expanded = false, weeks = expanded ? GITHUB_TRACKER_WEEKS : GITHUB_TRACKER_CARD_WEEKS }: { days: GitHubTrackerState["days"]; expanded?: boolean; weeks?: number }) {
  const cellCount = weeks * 7;
  const availableDays = days.slice(-cellCount);
  const emptyCount = Math.max(0, cellCount - availableDays.length);
  const cells = [
    ...Array.from({ length: emptyCount }, (_, index) => ({ date: `empty-${index}`, count: 0, level: 0 })),
    ...availableDays,
  ];
  const heatmap = (
    <div className={`githubHeatmap ${expanded ? "githubHeatmapExpanded" : ""}`} style={{ gridTemplateColumns: `repeat(${weeks}, var(--github-cell))` }} role="img" aria-label={`Публичная GitHub-активность за ${weeks} недель`}>
      {cells.map((day) => <i className={`githubHeatmapLevel${Math.max(0, Math.min(4, day.level))}`} key={day.date} title={day.date.startsWith("empty-") ? "" : `${formatStatisticsDay(day.date)}: ${day.count} коммитов`} />)}
    </div>
  );
  return expanded ? <div className="githubHeatmapViewport">{heatmap}</div> : heatmap;
}

function TrackerSlidersIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M4 7h9M17 7h3M4 17h3M11 17h9M13 4v6M7 14v6" />
    </svg>
  );
}

function TrackerStatisticsIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M4 19V5M4 19h16M7 15l4-4 3 2 5-7" />
      <path d="M16 6h3v3" />
    </svg>
  );
}

function WaterIcon() {
  return <img className="trackerBuiltinIcon" src="/tracker-builtins/water-transparent.png" alt="" aria-hidden="true" draggable={false} />;
}

function FlameIcon() {
  return <img className="trackerBuiltinIcon" src="/tracker-builtins/calories-transparent.png" alt="" aria-hidden="true" draggable={false} />;
}

function DumbbellIcon() {
  return <img className="trackerBuiltinIcon" src="/tracker-builtins/weight-transparent.png" alt="" aria-hidden="true" draggable={false} />;
}

function RingChart({ progress }: { progress: number }) {
  const circumference = 2 * Math.PI * 24;
  const offset = circumference * (1 - progress / 100);
  return (
    <svg className="trackerRing" viewBox="0 0 60 60" aria-hidden="true">
      <circle cx="30" cy="30" r="24" pathLength="100" />
      <circle className="trackerRingValue" cx="30" cy="30" r="24" pathLength="100" strokeDasharray="100" strokeDashoffset={offset / circumference * 100} />
    </svg>
  );
}

function WeightLine() {
  return (
    <svg className="weightLine" viewBox="0 0 100 64" aria-hidden="true">
      <path d="M3 56c9-27 18-23 27-15 8 7 16 4 20-14 5-24 16-29 23-2 5 20 10 19 16 4 3-7 6-8 9-2" />
    </svg>
  );
}

function Tab({
  active,
  icon,
  label,
  meta,
  onClick,
}: {
  active: boolean;
  icon: React.ReactNode;
  label: string;
  meta?: string;
  onClick: () => void;
}) {
  return (
    <button
      className={`tab ${active ? "tabActive" : ""}`}
      onClick={onClick}
      aria-current={active ? "page" : undefined}
    >
      <span className="tabIcon" aria-hidden="true">{icon}</span>
      <span className="tabLabel">{label}</span>
      {meta && <span className="tabMeta">{meta}</span>}
    </button>
  );
}

function bottomNavigationLabel(item: BottomNavigationItem) {
  return BOTTOM_NAVIGATION_OPTIONS.find((option) => option.id === item)?.label ?? item;
}

function BottomNavigationIcon({ item }: { item: BottomNavigationItem }) {
  if (item === "card") return <CardNavIcon />;
  if (item === "tasks") return <TasksNavIcon />;
  if (item === "projects") return <ProjectsNavIcon />;
  if (item === "tracker") return <TimeTrackerNavIcon />;
  if (item === "calories") return <FlameIcon />;
  return <ProfileNavIcon />;
}

function CardNavIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <rect x="3" y="5" width="18" height="14" rx="3" />
      <circle cx="8" cy="11" r="2" />
      <path d="M12.5 10h5M12.5 14h4" />
    </svg>
  );
}

function TasksNavIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="12" r="9" />
      <path d="m8 12 2.5 2.5L16.5 9" />
    </svg>
  );
}

function ProjectsNavIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M4 7.5h6l1.5 2H20v9.5H4z" />
      <path d="M4 7.5V5h6l1.5 2H20v2.5" />
    </svg>
  );
}

function ProfileNavIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="12" cy="8" r="3.5" />
      <path d="M5 20c.5-4.2 3-6.3 7-6.3s6.5 2.1 7 6.3" />
    </svg>
  );
}

function formatDate(value: string) {
  if (!value) return "—";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value.slice(0, 10);
  return parsed.toLocaleDateString("ru-RU", { day: "2-digit", month: "2-digit", year: "numeric" });
}

function formatRegisterDate(value: string) {
  if (!value) return "";
  const parsed = new Date(`${value.slice(0, 10)}T12:00:00`);
  if (Number.isNaN(parsed.getTime())) return value;
  const formatted = parsed.toLocaleDateString("ru-RU", {
    weekday: "long",
    day: "numeric",
    month: "long",
  });
  return formatted.charAt(0).toUpperCase() + formatted.slice(1);
}

function formatMonthYear(value: string) {
  const parsed = parseDateKey(value);
  if (!parsed) return "Календарь";
  const month = parsed.toLocaleDateString("ru-RU", { month: "long" });
  const label = `${month} ${parsed.getFullYear()}`;
  return label.charAt(0).toUpperCase() + label.slice(1);
}

function formatTaskDueDate(value: string, currentDate: string) {
  if (value === currentDate) return "Сегодня";
  if (value === addDaysToDateKey(currentDate, 1)) return "Завтра";
  if (value === addDaysToDateKey(currentDate, -1)) return "Вчера";
  const parsed = parseDateKey(value);
  if (!parsed) return value;
  return parsed.toLocaleDateString("ru-RU", {
    day: "numeric",
    month: "short",
    ...(parsed.getFullYear() === parseDateKey(currentDate)?.getFullYear() ? {} : { year: "numeric" }),
  });
}

function formatOverdueTaskDate(value: string, currentDate: string) {
  if (value === addDaysToDateKey(currentDate, -1)) return "вчера";
  const parsed = parseDateKey(value);
  if (!parsed) return value;
  return parsed.toLocaleDateString("ru-RU", { day: "numeric", month: "long" });
}

function formatSelectedDate(value: string) {
  const parsed = parseDateKey(value);
  if (!parsed) return value;
  const label = parsed.toLocaleDateString("ru-RU", { weekday: "long", day: "numeric", month: "long" });
  return label.charAt(0).toUpperCase() + label.slice(1);
}

function activeTaskLabel(count: number) {
  const lastTwo = count % 100;
  if (lastTwo >= 11 && lastTwo <= 14) return "активных";
  const last = count % 10;
  if (last === 1) return "активная";
  if (last >= 2 && last <= 4) return "активные";
  return "активных";
}

function formatValue(value: number) {
  return value.toLocaleString("ru-RU", { maximumFractionDigits: 2 });
}

function IDCard({
  profile,
  pinned,
  photoProcessing,
  photoProgress,
  onEdit,
  onSignatureClick,
  onPhotoClick,
  onTimeline,
}: {
  profile: Profile;
  pinned: Goal[];
  photoProcessing: boolean;
  photoProgress: number | null;
  onEdit: (field: ProfileField) => void;
  onSignatureClick: () => void;
  onPhotoClick: () => void;
  onTimeline: () => void;
}) {
  const field = (value: string, fallback: string) => value.trim() || fallback;
  return (
    <section className="idCardBlock">
      <div className="cardHeader">
        <div className="cardContext">
          <strong>PORTFOLIO ID</strong>
          <span>{pinned.length}/5 достижений закреплено</span>
        </div>
        {photoProcessing && (
          <span className="photoStatus">ОБРАБОТКА ФОТО{photoProgress === null ? "…" : ` · ${photoProgress}%`}</span>
        )}
      </div>

      <div className="cardViewport">
        <div className="card" role="group" aria-label="Портфолио-карта">
          <img
            className="cardArtwork"
            src="/card-front.png?v=4"
            width="2008"
            height="1276"
            alt=""
            aria-hidden="true"
            loading="eager"
            decoding="sync"
            fetchPriority="high"
            draggable={false}
          />
        <button className="cardData cardSurname" onClick={() => onEdit("surname")} aria-label="Изменить фамилию">{field(profile.surname, "ФАМИЛИЯ")}</button>
        <button className="cardData cardGivenName" onClick={() => onEdit("name")} aria-label="Изменить имя">{field(profile.name, "ИМЯ")}</button>
        <button className="cardData cardSex" onClick={() => onEdit("sex")} aria-label="Изменить пол">{field(profile.sex, "—")}</button>
        <button className="cardData cardOccupation" onClick={() => onEdit("occupation")} aria-label="Изменить род занятий">{field(profile.occupation, "—")}</button>
        <button className="cardData cardDob" onClick={() => onEdit("dob")} aria-label="Изменить дату рождения">{field(profile.dob, "—")}</button>
        <output className="cardData cardExpiry cardDataFixed" aria-label={`Карта действует до ${CARD_EXPIRY_DATE}`}>{CARD_EXPIRY_DATE}</output>
        <button className={`cardSignature ${profile.signature ? "cardSignatureDrawn" : ""}`} onClick={onSignatureClick} aria-label={profile.signature ? "Изменить подпись" : "Добавить подпись"}>
          {profile.signature
            ? <img src={profile.signature} alt="Подпись владельца карты" draggable={false} />
            : <span>{`${profile.name} ${profile.surname}`.trim().toLowerCase()}</span>}
        </button>

        <button className="cardPhoto" onClick={onPhotoClick} disabled={photoProcessing} title="Редактировать фотографию">
          {profile.photo && <img src={profile.photo} alt="Фото владельца карты" />}
          <span className="cardPhotoHint" aria-hidden="true">{photoProcessing ? "ОБРАБОТКА…" : "✎"}</span>
        </button>

        {pinned.length > 0 && <button className="cardAchievements" type="button" onClick={onTimeline} aria-label="Открыть полный таймлайн портфолио">
          {pinned.map((goal) => (
            <div className="cardAchievement" key={goal.id}>
              <time>{formatDate(goal.completedAt).slice(0, 5)}</time>
              <div>
                <span>{goal.title}</span>
                {goal.summary && <small>{goal.summary}</small>}
              </div>
            </div>
          ))}
        </button>}
      </div>
      </div>
      <p className="cardMobileNote">Нажмите на данные, фото или подпись для редактирования.{pinned.length > 0 ? " Нажмите на достижения, чтобы открыть таймлайн." : ""}</p>
    </section>
  );
}

function reminderToInput(value: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function reminderToRFC3339(value: string) {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : date.toISOString();
}

const TASK_PRIORITY_OPTIONS = [
  { value: 3, label: "Высокий приоритет" },
  { value: 2, label: "Средний приоритет" },
  { value: 1, label: "Низкий приоритет" },
] as const;

function TaskCategoryPicker({
  value,
  categories,
  onChange,
  onCreate,
  onDelete,
}: {
  value: string;
  categories: TaskCategory[];
  onChange: (value: string) => void;
  onCreate: (name: string) => Promise<string>;
  onDelete?: (category: TaskCategory) => Promise<boolean>;
}) {
  const [open, setOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState("");
  const [busy, setBusy] = useState(false);
  const [deletingID, setDeletingID] = useState<number | null>(null);
  const [error, setError] = useState("");
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
        setCreateOpen(false);
        setError("");
      }
    };
    window.addEventListener("pointerdown", close);
    return () => window.removeEventListener("pointerdown", close);
  }, [open]);

  function choose(next: string) {
    onChange(next);
    setOpen(false);
    setCreateOpen(false);
    setError("");
  }

  async function create() {
    const name = newName.trim();
    if (!name || busy) return;
    const existing = categories.find((item) => item.name.localeCompare(name, "ru", { sensitivity: "accent" }) === 0);
    if (existing) {
      choose(existing.name);
      setNewName("");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const savedName = await onCreate(name);
      choose(savedName);
      setNewName("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось создать категорию");
    } finally {
      setBusy(false);
    }
  }

  async function remove(category: TaskCategory) {
    if (!onDelete || category.builtin || category.id <= 0 || deletingID !== null) return;
    setDeletingID(category.id);
    setError("");
    try {
      const deleted = await onDelete(category);
      if (deleted && value.localeCompare(category.name, "ru", { sensitivity: "accent" }) === 0) onChange("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось удалить категорию");
    } finally {
      setDeletingID(null);
    }
  }

  return (
    <div className="taskPicker" ref={rootRef}>
      <button type="button" className={`taskPickerButton ${open ? "isOpen" : ""}`} onClick={() => { setOpen((valueOpen) => !valueOpen); setCreateOpen(false); setError(""); }} aria-haspopup="listbox" aria-expanded={open}>
        <span>{value.trim() || "Все"}</span><b aria-hidden="true">⌄</b>
      </button>
      {open && <div className="taskPickerMenu taskCategoryPickerMenu" role="listbox" aria-label="Категория задачи">
        <button type="button" className={!value.trim() ? "isSelected" : ""} role="option" aria-selected={!value.trim()} onClick={() => choose("")}><span>Все</span><b>{!value.trim() ? "✓" : ""}</b></button>
        {categories.map((category) => <div className="taskCategoryOptionRow" key={category.id || category.name}>
          <button type="button" className={value === category.name ? "isSelected" : ""} role="option" aria-selected={value === category.name} onClick={() => choose(category.name)}><span>{category.name}</span><b>{value === category.name ? "✓" : ""}</b></button>
          {onDelete && !category.builtin && category.id > 0 && <button type="button" className="taskCategoryDeleteAction" disabled={deletingID !== null} onClick={(event) => { event.preventDefault(); event.stopPropagation(); void remove(category); }} aria-label={`Удалить категорию ${category.name}`}>×</button>}
        </div>)}
        <button type="button" className="taskPickerCreate" onClick={() => { setCreateOpen(true); setError(""); }}><i aria-hidden="true">＋</i><span>Создать категорию</span></button>
        {createOpen && <div className="taskPickerCreateForm">
          <input value={newName} maxLength={40} autoFocus placeholder="Название категории" onChange={(event) => setNewName(event.target.value)} onKeyDown={(event) => { if (event.key !== "Enter") return; event.preventDefault(); event.stopPropagation(); void create(); }} />
          <button type="button" disabled={!newName.trim() || busy} onClick={(event) => { event.preventDefault(); event.stopPropagation(); void create(); }}>{busy ? "…" : "Создать"}</button>
          {error && <small>{error}</small>}
        </div>}
      </div>}
    </div>
  );
}

function TaskPriorityPicker({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const selected = TASK_PRIORITY_OPTIONS.find((item) => item.value === value);

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    window.addEventListener("pointerdown", close);
    return () => window.removeEventListener("pointerdown", close);
  }, [open]);

  function choose(next: number) {
    onChange(next);
    setOpen(false);
  }

  return (
    <div className="taskPicker taskPrioritySelect" ref={rootRef}>
      <button type="button" className={`taskPickerButton ${open ? "isOpen" : ""}`} onClick={() => setOpen((valueOpen) => !valueOpen)} aria-haspopup="listbox" aria-expanded={open}>
        <span className="taskPriorityCurrent">{selected && <i className={`taskPriorityDot priority-${selected.value}`} aria-hidden="true" />}<span>{selected?.label ?? "Без приоритета"}</span></span><b aria-hidden="true">⌄</b>
      </button>
      {open && <div className="taskPickerMenu taskPriorityMenu" role="listbox" aria-label="Приоритет задачи">
        {TASK_PRIORITY_OPTIONS.map((item) => <button type="button" className={value === item.value ? "isSelected" : ""} role="option" aria-selected={value === item.value} key={item.value} onClick={() => choose(item.value)}><i className={`taskPriorityDot priority-${item.value}`} aria-hidden="true" /><span>{item.label}</span><b>{value === item.value ? "✓" : ""}</b></button>)}
        {value !== 0 && <button type="button" className="taskPriorityClear" onClick={() => choose(0)}><span>Без приоритета</span></button>}
      </div>}
    </div>
  );
}

function categoryMatches(task: Task, category: string | null) {
  return category === null || task.category.localeCompare(category, "ru", { sensitivity: "accent" }) === 0;
}

function TasksPage({
  tasks,
  projects,
  currentDate,
  onCreate,
  onEdit,
  onStatus,
  onMoveToToday,
  onMoveInDay,
  orderBusy,
  onDelete,
}: {
  tasks: Task[];
  projects: Goal[];
  currentDate: string;
  onCreate: (input: TaskInput) => Promise<boolean>;
  onEdit: (task: Task) => void;
  onStatus: (task: Task, status: TaskStatus) => Promise<void>;
  onMoveToToday: (task: Task, date: string) => Promise<void>;
  onMoveInDay: (task: Task, otherTask: Task) => Promise<void>;
  orderBusy: boolean;
  onDelete: (task: Task) => Promise<void>;
}) {
  const [todayKey, setTodayKey] = useState(() => formalTodayKey(currentDate || localTodayKey()));
  const [desktopTaskLayout] = useState(() => window.matchMedia("(min-width: 960px) and (hover: hover) and (pointer: fine)").matches);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [category, setCategory] = useState("");
  const [dueDate, setDueDate] = useState(todayKey);
  const [dueTime, setDueTime] = useState("");
  const [reminderLocal, setReminderLocal] = useState("");
  const [priority, setPriority] = useState(0);
  const [recurrenceType, setRecurrenceType] = useState<TaskRecurrenceType>("");
  const [recurrenceInterval, setRecurrenceInterval] = useState(1);
  const [recurrenceEndDate, setRecurrenceEndDate] = useState("");
  const [recurrenceWeekdays, setRecurrenceWeekdays] = useState<number[]>([]);
  const [projectIDs, setProjectIDs] = useState<number[]>([]);
  const [selectedDate, setSelectedDate] = useState<string | null>(() => desktopTaskLayout ? null : todayKey);
  const [selectedCategory, setSelectedCategory] = useState<string | null>(null);
  const [weekStart, setWeekStart] = useState(startOfWeekDateKey(todayKey));
  const [filterOpen, setFilterOpen] = useState(false);
  const [monthOpen, setMonthOpen] = useState(false);
  const [monthCursor, setMonthCursor] = useState(startOfMonthDateKey(todayKey));
  const [createOpen, setCreateOpen] = useState(false);
  const [createSubmitting, setCreateSubmitting] = useState(false);
  const [categories, setCategories] = useState<TaskCategory[]>([]);
  const [newCategory, setNewCategory] = useState("");
  const [categoryBusy, setCategoryBusy] = useState(false);
  const [completedOpen, setCompletedOpen] = useState(true);
  const [notificationConfig, setNotificationConfig] = useState<{ configured: boolean; publicKey: string } | null>(null);
  const [notificationState, setNotificationState] = useState<DeviceNotificationState>("unknown");
  const createKeyboardOpen = useVirtualKeyboardOpen();
  const filterMenuRef = useRef<HTMLDivElement>(null);
  const taskDocumentRef = useRef<HTMLElement>(null);
  const taskBoardRef = useRef<HTMLDivElement>(null);
  const skipInitialDesktopTodaySelection = useRef(desktopTaskLayout);

  useEffect(() => {
    const updateFormalDate = () => setTodayKey(formalTodayKey(currentDate || localTodayKey()));
    updateFormalDate();
    const timer = window.setInterval(updateFormalDate, 60_000);
    document.addEventListener("visibilitychange", updateFormalDate);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", updateFormalDate);
    };
  }, [currentDate]);

  const visibleWeekStart = weekStart || startOfWeekDateKey(todayKey);
  const calendarLabelDate = selectedDate ?? addDaysToDateKey(visibleWeekStart, 3);
  const allTasksView = selectedDate === null && selectedCategory === null;
  const desktopAllTasksBoard = desktopTaskLayout && allTasksView;
  const availableCategories = useMemo(() => categories, [categories]);
  const availableProjects = useMemo(() => projects.filter((project) => !project.completed), [projects]);
  const weekDays = useMemo(() => Array.from({ length: 7 }, (_, index) => {
    const date = addDaysToDateKey(visibleWeekStart, index);
    const parsed = parseDateKey(date);
    return {
      date,
      day: parsed?.getDate() ?? index + 1,
      weekday: parsed?.toLocaleDateString("ru-RU", { weekday: "short" }).replace(".", "") ?? "",
      taskCount: tasks.filter((task) => task.status !== "done" && task.dueDate === date && categoryMatches(task, selectedCategory)).length,
    };
  }), [tasks, visibleWeekStart, selectedCategory]);

  const visibleTasks = useMemo(() => {
    const filtered = tasks.filter((task) => task.status === "todo" && categoryMatches(task, selectedCategory));
    const result = selectedDate
      ? filtered.filter((task) => task.dueDate === selectedDate || (selectedDate === todayKey && Boolean(task.dueDate) && task.dueDate < todayKey))
      : filtered;
    return [...result].sort((left, right) => {
      const dateOrder = (left.dueDate || "9999-12-31").localeCompare(right.dueDate || "9999-12-31");
      return dateOrder || left.sortOrder - right.sortOrder || left.title.localeCompare(right.title, "ru");
    });
  }, [tasks, selectedCategory, selectedDate, todayKey]);

  const taskSections = useMemo(() => {
    if (desktopAllTasksBoard) {
      const tomorrowKey = addDaysToDateKey(todayKey, 1);
      const nextSevenDaysEnd = addDaysToDateKey(todayKey, 7);
      const overdueTasks = visibleTasks.filter((task) => Boolean(task.dueDate) && task.dueDate < todayKey);
      const todayTasks = visibleTasks.filter((task) => task.dueDate === todayKey);
      return [
        {
          key: "overdue",
          title: "Просрочено",
          tasks: overdueTasks,
          addDate: addDaysToDateKey(todayKey, -1),
        },
        {
          key: "today",
          title: "Сегодня",
          tasks: todayTasks,
          addDate: todayKey,
        },
        {
          key: "tomorrow",
          title: "Завтра",
          tasks: visibleTasks.filter((task) => task.dueDate === tomorrowKey),
          addDate: tomorrowKey,
        },
        {
          key: "next-seven-days",
          title: "Следующие 7 дней",
          tasks: visibleTasks.filter((task) => Boolean(task.dueDate) && task.dueDate > tomorrowKey && task.dueDate <= nextSevenDaysEnd),
          addDate: addDaysToDateKey(todayKey, 2),
        },
        {
          key: "later",
          title: "Позже",
          tasks: visibleTasks.filter((task) => !task.dueDate || task.dueDate > nextSevenDaysEnd),
          addDate: addDaysToDateKey(todayKey, 8),
        },
      ].filter((section) => section.tasks.length > 0);
    }
    if (!selectedDate) {
      const grouped = new Map<string, Task[]>();
      visibleTasks.forEach((task) => {
        const key = task.dueDate || "undated";
        grouped.set(key, [...(grouped.get(key) ?? []), task]);
      });
      return [...grouped.entries()].map(([date, items]) => ({
        key: date,
        title: date === "undated" ? "Без даты" : date === todayKey ? "Сегодня" : formatSelectedDate(date),
        tasks: items,
        addDate: date === "undated" ? todayKey : date,
      }));
    }
    const overdueTasks = visibleTasks.filter((task) => Boolean(task.dueDate) && task.dueDate < todayKey);
    const currentTasks = visibleTasks.filter((task) => !task.dueDate || task.dueDate >= todayKey);
    return [
      ...(overdueTasks.length > 0 ? [{ key: "overdue", title: "Просроченные", tasks: overdueTasks, addDate: todayKey }] : []),
      ...(currentTasks.length > 0 ? [{ key: "todo", title: "Невыполненные", tasks: currentTasks, addDate: selectedDate }] : []),
    ];
  }, [desktopAllTasksBoard, selectedDate, todayKey, visibleTasks]);

  const completedTasksForDay = useMemo(() => {
    if (!selectedDate) return [];
    return tasks
      .filter((task) => task.status === "done" && categoryMatches(task, selectedCategory) && dateKeyFromTimestamp(task.completedAt) === selectedDate)
      .sort((left, right) => right.completedAt.localeCompare(left.completedAt));
  }, [tasks, selectedCategory, selectedDate]);

  const activeCount = tasks.filter((task) => task.status === "todo" && task.dueDate === todayKey).length;
  const allActiveCount = tasks.filter((task) => task.status === "todo").length;

  useEffect(() => {
    if (skipInitialDesktopTodaySelection.current) {
      skipInitialDesktopTodaySelection.current = false;
    } else {
      setSelectedDate(todayKey);
    }
    setWeekStart(startOfWeekDateKey(todayKey));
  }, [todayKey]);

  useEffect(() => {
    setCompletedOpen(true);
  }, [selectedDate]);

  useEffect(() => {
    const board = taskBoardRef.current;
    const wheelArea = taskDocumentRef.current;
    if (selectedDate !== null || selectedCategory !== null || !board || !wheelArea) return;
    let targetScrollLeft = board.scrollLeft;
    let animationFrame: number | null = null;

    const animateBoard = () => {
      const distance = targetScrollLeft - board.scrollLeft;
      if (Math.abs(distance) < 0.5) {
        board.scrollLeft = targetScrollLeft;
        animationFrame = null;
        return;
      }
      board.scrollLeft += distance * 0.18;
      animationFrame = window.requestAnimationFrame(animateBoard);
    };

    const scrollBoardWithWheel = (event: WheelEvent) => {
      if (window.innerWidth < 960) return;
      if (!(event.target instanceof Node) || !wheelArea.contains(event.target)) return;
      if (board.scrollWidth <= board.clientWidth) return;

      // Обычное вертикальное колесо мыши управляет горизонтальной доской.
      // Горизонтальный жест трекпада обрабатываем тем же способом, чтобы
      // поведение не зависело от браузера и типа устройства ввода.
      const rawDelta = Math.abs(event.deltaY) >= Math.abs(event.deltaX) ? event.deltaY : event.deltaX;
      if (rawDelta === 0) return;
      const multiplier = event.deltaMode === 1
        ? 36
        : event.deltaMode === 2 ? board.clientWidth : 1;
      const normalizedDelta = rawDelta * multiplier;
      const scrollDelta = normalizedDelta * 1.25;
      const maxScrollLeft = board.scrollWidth - board.clientWidth;
      if (animationFrame === null) targetScrollLeft = board.scrollLeft;
      const nextScrollLeft = Math.max(0, Math.min(maxScrollLeft, targetScrollLeft + scrollDelta));

      // На краях доски возвращаем странице обычную вертикальную прокрутку.
      if (Math.abs(nextScrollLeft - targetScrollLeft) < 0.5 && Math.abs(targetScrollLeft - board.scrollLeft) < 0.5) return;
      event.preventDefault();
      targetScrollLeft = nextScrollLeft;
      if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
        board.scrollLeft = targetScrollLeft;
        return;
      }
      if (animationFrame === null) animationFrame = window.requestAnimationFrame(animateBoard);
    };
    window.addEventListener("wheel", scrollBoardWithWheel, { capture: true, passive: false });
    return () => {
      window.removeEventListener("wheel", scrollBoardWithWheel, { capture: true });
      if (animationFrame !== null) window.cancelAnimationFrame(animationFrame);
    };
  }, [selectedCategory, selectedDate, taskSections.length]);

  useEffect(() => {
    let active = true;
    Promise.all([api.taskCategories(), api.notificationConfig()])
      .then(([loadedCategories, config]) => {
        if (!active) return;
        setCategories(loadedCategories);
        setNotificationConfig(config);
        if (!("Notification" in window) || !("serviceWorker" in navigator) || !("PushManager" in window)) {
          setNotificationState("unsupported");
        } else if (Notification.permission === "denied") {
          setNotificationState("denied");
        } else if (Notification.permission === "granted") {
          setNotificationState("enabled");
          void syncPushSubscription(config, false);
        }
      })
      .catch(() => setNotificationState("error"));
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (!filterOpen) return;
    const close = (event: PointerEvent) => {
      if (!filterMenuRef.current?.contains(event.target as Node)) setFilterOpen(false);
    };
    window.addEventListener("pointerdown", close);
    return () => window.removeEventListener("pointerdown", close);
  }, [filterOpen]);

  useEffect(() => {
    if (!createOpen) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => { document.body.style.overflow = previous; };
  }, [createOpen]);

  async function syncPushSubscription(config = notificationConfig, requestPermission = true) {
    const state = await registerDeviceNotifications(requestPermission, config ?? undefined);
    setNotificationState(state);
    return state === "enabled";
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (createSubmitting || !title.trim() || (!dueDate && !category.trim())) return;
    setCreateSubmitting(true);
    setCreateOpen(false);
    const created = await onCreate({
      title: title.trim(),
      description: description.trim(),
      category: category.trim(),
      status: "todo",
      dueDate,
      dueTime: dueDate ? dueTime : "",
      reminderAt: reminderToRFC3339(reminderLocal),
      priority,
      isMilestone: priority === 3,
      recurrenceType,
      recurrenceInterval,
      recurrenceEndDate,
      recurrenceWeekdays,
      projectIds: projectIDs,
    });
    setCreateSubmitting(false);
    if (!created) {
      setCreateOpen(true);
      return;
    }
    setTitle("");
    setDescription("");
    setCategory("");
    setDueTime("");
    setReminderLocal("");
    setPriority(0);
    setRecurrenceType("");
    setRecurrenceInterval(1);
    setRecurrenceEndDate("");
    setRecurrenceWeekdays([]);
    setProjectIDs([]);
    setCreateOpen(false);
  }

  async function addCategory(event: React.FormEvent) {
    event.preventDefault();
    const name = newCategory.trim();
    if (!name || categoryBusy) return;
    setCategoryBusy(true);
    try {
      const saved = await api.createTaskCategory(name);
      setCategories((items) => [...items.filter((item) => item.id !== saved.id), saved]);
      setSelectedCategory(saved.name);
      setCategory(saved.name);
      setSelectedDate(null);
      setNewCategory("");
    } finally {
      setCategoryBusy(false);
    }
  }

  async function createCategoryForTask(name: string) {
    const saved = await api.createTaskCategory(name);
    setCategories((items) => [...items.filter((item) => item.id !== saved.id), saved]);
    return saved.name;
  }

  async function deleteCategory(categoryToDelete: TaskCategory) {
    if (categoryToDelete.builtin) return false;
    const activeCount = tasks.filter((task) => task.status !== "done" && categoryMatches(task, categoryToDelete.name)).length;
    if (activeCount > 0) {
      window.alert(`Категорию «${categoryToDelete.name}» нельзя удалить: в ней ${activeCount} активных задач. Сначала завершите или перенесите их.`);
      return false;
    }
    if (!window.confirm(`Удалить категорию «${categoryToDelete.name}»? Выполненные задачи сохранят своё название категории.`)) return false;
    try {
      await api.deleteTaskCategory(categoryToDelete.id);
      setCategories((items) => items.filter((item) => item.id !== categoryToDelete.id));
      if (selectedCategory && selectedCategory.localeCompare(categoryToDelete.name, "ru", { sensitivity: "accent" }) === 0) setSelectedCategory(null);
      if (category && category.localeCompare(categoryToDelete.name, "ru", { sensitivity: "accent" }) === 0) setCategory("");
      return true;
    } catch (cause) {
      window.alert(cause instanceof Error ? cause.message : "Не удалось удалить категорию");
      return false;
    }
  }

  function chooseDate(date: string) {
    setSelectedDate(date);
    setDueDate(date);
    setFilterOpen(false);
    setMonthOpen(false);
  }

  function showToday() {
    setSelectedDate(todayKey);
    setDueDate(todayKey);
    setWeekStart(startOfWeekDateKey(todayKey));
    setMonthOpen(false);
  }

  function chooseCategory(value: string | null) {
    setSelectedCategory(value);
    setSelectedDate(null);
    setFilterOpen(false);
    setMonthOpen(false);
  }

  function toggleAllTasks() {
    if (selectedDate === null && selectedCategory === null) {
      showToday();
      return;
    }
    setSelectedCategory(null);
    setSelectedDate(null);
    setFilterOpen(false);
    setMonthOpen(false);
  }

  function openCreateForm(initialDueDate = selectedDate ?? todayKey) {
    if (createSubmitting) return;
    setCategory("");
    setPriority(0);
    setRecurrenceType("");
    setRecurrenceInterval(1);
    setRecurrenceEndDate("");
    setRecurrenceWeekdays([]);
    setProjectIDs([]);
    setDueDate(initialDueDate);
    setCreateOpen(true);
  }

  const filterLabel = selectedCategory ?? "Категория";
  const emptyText = selectedDate
    ? `На ${formatSelectedDate(selectedDate).toLocaleLowerCase("ru-RU")} задач нет.`
    : selectedCategory
      ? `В категории «${selectedCategory}» нет активных задач.`
      : "Активных задач нет.";

  return (
    <div className="taskDesktopWorkspace">
      <aside className="taskDesktopSidebar" aria-label="Навигация по задачам">
        <header className="taskDesktopSidebarHeader">
          <span>Рабочее пространство</span>
          <strong>Задачи</strong>
        </header>
        <nav className="taskDesktopNav" aria-label="Списки задач">
          <button type="button" className={selectedDate === null && selectedCategory === null ? "isActive" : ""} aria-current={selectedDate === null && selectedCategory === null ? "page" : undefined} onClick={() => chooseCategory(null)}>
            <span className="taskDesktopNavIcon"><AllTasksIcon /></span><span>Все</span><small>{allActiveCount}</small>
          </button>
          <button type="button" className={selectedDate === todayKey && selectedCategory === null ? "isActive" : ""} aria-current={selectedDate === todayKey && selectedCategory === null ? "page" : undefined} onClick={showToday}>
            <span className="taskDesktopNavIcon taskDesktopTodayIcon" aria-hidden="true">{parseDateKey(todayKey)?.getDate() ?? "•"}</span><span>Сегодня</span><small>{activeCount}</small>
          </button>
        </nav>
        <div className="taskDesktopCategories">
          <div className="taskDesktopSidebarLabel"><span>Категории</span><i /></div>
          <nav aria-label="Категории задач">
            {availableCategories.map((categoryItem) => (
              <div className="taskDesktopCategoryRow" key={categoryItem.id}>
                <button type="button" className={selectedCategory === categoryItem.name ? "isActive" : ""} aria-current={selectedCategory === categoryItem.name ? "page" : undefined} onClick={() => chooseCategory(categoryItem.name)}>
                  <span className="taskDesktopCategoryMark" aria-hidden="true">{categoryItem.name.slice(0, 1).toUpperCase()}</span>
                  <span>{categoryItem.name}</span>
                  <small>{tasks.filter((task) => task.status === "todo" && categoryMatches(task, categoryItem.name)).length}</small>
                </button>
                {!categoryItem.builtin && <button type="button" className="taskDesktopCategoryDelete" onClick={() => void deleteCategory(categoryItem)} aria-label={`Удалить категорию ${categoryItem.name}`}>×</button>}
              </div>
            ))}
          </nav>
          <form className="taskDesktopCategoryCreate" noValidate onSubmit={addCategory}>
            <input value={newCategory} maxLength={40} placeholder="Новая категория" aria-label="Название новой категории" onChange={(event) => setNewCategory(event.target.value)} />
            <button type="submit" disabled={!newCategory.trim() || categoryBusy} aria-label="Создать категорию">＋</button>
          </form>
        </div>
        <button type="button" className="taskDesktopCreate" disabled={createSubmitting} aria-busy={createSubmitting} onClick={() => openCreateForm()}><span aria-hidden="true">＋</span>{createSubmitting ? "Создание…" : "Новая задача"}</button>
      </aside>

      <section ref={taskDocumentRef} className={`doc taskDocument ${selectedDate === null && selectedCategory === null ? "taskDocumentAll" : ""}`}>
      <header className="taskHeader">
        <div>
          <span className="taskKicker">Личный список</span>
          <h1>{desktopAllTasksBoard ? "Все задачи" : "Задачи"}</h1>
          <span className="taskDesktopCurrentView">{selectedCategory ?? (selectedDate === null ? "Все" : selectedDate === todayKey ? "Сегодня" : formatSelectedDate(selectedDate))}</span>
          <time dateTime={todayKey}>{formatRegisterDate(todayKey)}</time>
        </div>
        <div className="taskHeaderMeta">
          <div className="taskOpenCount" aria-label={`${desktopAllTasksBoard ? allActiveCount : activeCount} активных задач`}>
            <strong>{desktopAllTasksBoard ? allActiveCount : activeCount}</strong><span>{activeTaskLabel(desktopAllTasksBoard ? allActiveCount : activeCount)}</span>
          </div>
        </div>
      </header>

      <section className="taskCalendar" aria-label="Календарь задач">
        <div className="taskCalendarTop">
          <button className={`taskMonthButton ${monthOpen ? "taskMonthButtonOpen" : ""}`} type="button" onClick={() => { setFilterOpen(false); setMonthOpen((open) => !open); setMonthCursor(startOfMonthDateKey(calendarLabelDate)); }} aria-expanded={monthOpen}>
            <span>Неделя</span><strong>{formatMonthYear(calendarLabelDate)}</strong>
          </button>
          <div className="taskCalendarModes">
            <div className="taskFilterControl" ref={filterMenuRef}>
              <button type="button" className={`taskCalendarModeButton ${selectedCategory !== null ? "isActive" : ""}`} onClick={() => { setMonthOpen(false); setFilterOpen((open) => !open); }} aria-label={`Категория: ${filterLabel}`} aria-expanded={filterOpen} title={`Категория: ${filterLabel}`}>
                <FilterIcon /><span>Категория</span>
              </button>
              {filterOpen && (
                <div className="taskFilterMenu taskCategoryMenu" role="menu" aria-label="Категории задач">
                  <header><span>Категории</span><small>{selectedCategory ?? "Выберите категорию"}</small></header>
                  {availableCategories.map((categoryItem) => (
                    <div className="taskCategoryMenuRow" key={categoryItem.id}>
                      <button type="button" role="menuitemradio" aria-checked={selectedCategory === categoryItem.name} onClick={() => chooseCategory(categoryItem.name)}><span>{categoryItem.name}</span><small>{tasks.filter((task) => task.status === "todo" && categoryMatches(task, categoryItem.name)).length}</small><b>{selectedCategory === categoryItem.name ? "✓" : ""}</b></button>
                      {!categoryItem.builtin && <button type="button" className="taskCategoryDeleteAction" onClick={(event) => { event.stopPropagation(); void deleteCategory(categoryItem); }} aria-label={`Удалить категорию ${categoryItem.name}`}>×</button>}
                    </div>
                  ))}
                  <form className="taskCategoryCreate" noValidate onSubmit={addCategory}><input value={newCategory} maxLength={40} placeholder="Новая категория" onChange={(event) => setNewCategory(event.target.value)} /><button disabled={!newCategory.trim() || categoryBusy}>＋</button></form>
                </div>
              )}
            </div>
            <button type="button" className={`taskCalendarModeButton ${selectedDate === null && selectedCategory === null ? "isActive" : ""}`} onClick={toggleAllTasks} aria-pressed={selectedDate === null && selectedCategory === null}>
              <AllTasksIcon /><span>Все задачи</span>
            </button>
          </div>
        </div>
        {monthOpen && (
          <div className="taskMonthPicker" role="dialog" aria-label="Выбор даты">
            <MonthCalendar monthDate={monthCursor || startOfMonthDateKey(calendarLabelDate)} selectedDate={selectedDate} currentDate={todayKey} tasks={tasks} onMove={(direction) => setMonthCursor((value) => addMonthsToDateKey(value || calendarLabelDate, direction))} onSelect={(date) => { setWeekStart(startOfWeekDateKey(date)); chooseDate(date); }} onToday={showToday} />
          </div>
        )}
        <div className="taskWeekScroller"><div className="taskWeek" role="group" aria-label="Дни недели">
          {weekDays.map((day) => {
            const selected = selectedDate === day.date;
            return <button key={day.date} type="button" className={`taskWeekDay ${selected ? "taskWeekDaySelected" : ""} ${day.date === todayKey ? "taskWeekDayToday" : ""}`} onClick={() => chooseDate(day.date)} aria-pressed={selected}>
              <strong>{day.day}</strong><span>{day.weekday}</span><i className={day.taskCount ? "hasTasks" : ""} aria-hidden="true" />
            </button>;
          })}
        </div></div>
      </section>


      {taskSections.length === 0 && completedTasksForDay.length === 0 ? <EmptyState title={tasks.length ? "Здесь пока пусто" : "Список пуст"} text={tasks.length ? emptyText : "Создайте первую задачу."} /> : (
        <div ref={taskBoardRef} className={`taskList taskListCompact ${selectedDate === null ? "taskListBoard" : ""} ${selectedDate === null && selectedCategory === null ? "taskListBoardAll" : ""}`}>
          {taskSections.map((section) => <section className={`taskSection taskSection-${section.key}${section.tasks.length === 0 ? " taskSectionEmpty" : ""}`} key={section.key}>
            <header className="taskSectionHeader"><span>{section.title}</span><small>{section.tasks.length}</small>{desktopAllTasksBoard && <button type="button" className="taskSectionAdd" onClick={() => openCreateForm(section.addDate)} aria-label={`Добавить задачу: ${section.title}`} title="Быстро добавить задачу">＋</button>}</header>
            {section.tasks.map((task, index) => <TaskRow
              key={task.id}
              task={task}
              currentDate={todayKey}
              moveUpTask={index > 0 && section.tasks[index - 1].dueDate === task.dueDate ? section.tasks[index - 1] : undefined}
              moveDownTask={index + 1 < section.tasks.length && section.tasks[index + 1].dueDate === task.dueDate ? section.tasks[index + 1] : undefined}
              onEdit={onEdit}
              onStatus={onStatus}
              onMoveToToday={onMoveToToday}
              onMoveInDay={onMoveInDay}
              orderBusy={orderBusy}
              showOverdueDate={section.key === "overdue"}
              boardCard={desktopAllTasksBoard}
              onDelete={onDelete}
            />)}
          </section>)}
          {selectedDate && completedTasksForDay.length > 0 && (
            <section className={`completedTaskPanel ${completedOpen ? "isOpen" : ""}`} aria-label={`Выполнено за день: ${completedTasksForDay.length}`}>
              <button type="button" className="completedTaskPanelHeader" onClick={() => setCompletedOpen((open) => !open)} aria-expanded={completedOpen}>
                <span>Выполнено</span><small>{completedTasksForDay.length}</small><b aria-hidden="true">⌄</b>
              </button>
              {completedOpen && <div className="completedTaskRows">{completedTasksForDay.map((task) => <TaskRow key={task.id} task={task} currentDate={todayKey} onEdit={onEdit} onStatus={onStatus} onMoveToToday={onMoveToToday} onDelete={onDelete} />)}</div>}
            </section>
          )}
        </div>
      )}

      <button type="button" className="createFab" disabled={createSubmitting} aria-busy={createSubmitting} onClick={() => openCreateForm()}><span aria-hidden="true">＋</span> {createSubmitting ? "Создание…" : "Задача"}</button>
      {createOpen && <Modal title="Новая задача" wide onClose={() => setCreateOpen(false)}>
        <form className={`mobileCreateForm taskEditorForm ${createKeyboardOpen ? "taskEditorKeyboardOpen" : ""}`} noValidate onSubmit={submit}>
          <label className="taskQuickTitle"><span>Что нужно сделать?</span><textarea className="resize-none" value={title} maxLength={160} autoFocus rows={2} placeholder="Название задачи" onChange={(event) => setTitle(event.target.value)} /></label>
          <div className="field taskDescriptionField"><span className="fieldLabel">Описание</span><TaskDescriptionEditor taskID={0} value={description} onChange={setDescription} /></div>
          <div className="taskFormGrid">
            <div className="field"><span className="fieldLabel">Категория</span><TaskCategoryPicker value={category} categories={availableCategories} onChange={setCategory} onCreate={createCategoryForTask} onDelete={deleteCategory} /></div>
            <div className="field"><span className="fieldLabel">Дата</span><TaskDateControl value={dueDate} currentDate={todayKey} allowUndated={Boolean(category.trim())} onChange={(value) => { setDueDate(value); if (!value) setDueTime(""); }} /></div>
            <div className="field"><span className="fieldLabel">Время задачи</span><TaskScheduleInput type="time" value={dueTime} disabled={!dueDate} onChange={setDueTime} /></div>
            <label className="field"><span className="fieldLabel">Напомнить</span><TaskScheduleInput type="datetime-local" value={reminderLocal} onChange={setReminderLocal} /></label>
          </div>
          <TaskRecurrenceControl dueDate={dueDate} type={recurrenceType} interval={recurrenceInterval} endDate={recurrenceEndDate} weekdays={recurrenceWeekdays} onType={setRecurrenceType} onInterval={setRecurrenceInterval} onEndDate={setRecurrenceEndDate} onWeekdays={setRecurrenceWeekdays} />
          <div className="field"><span className="fieldLabel">Приоритет</span><TaskPriorityPicker value={priority} onChange={setPriority} /></div>
          {reminderLocal && notificationState !== "enabled" && <div className="notificationSetup"><div><strong>Уведомления на устройство</strong><span>{notificationState === "denied" ? "Разрешение заблокировано в настройках браузера." : notificationState === "unsupported" ? "Этот браузер или режим не поддерживает Web Push." : "Разрешите уведомления, чтобы напоминание пришло даже при закрытом приложении."}</span></div>{!(["denied", "unsupported"] as string[]).includes(notificationState) && <button type="button" className="secondaryButton" onClick={() => void syncPushSubscription()}>Включить</button>}</div>}
          <fieldset className="relatedPicker taskProjectPicker">
            <legend>Прикрепить к проектам</legend>
            {availableProjects.length === 0 ? <p>Незавершённых проектов пока нет.</p> : <div>{availableProjects.map((project) => (
              <label key={project.id} className={projectIDs.includes(project.id) ? "selected" : ""}>
                <input type="checkbox" checked={projectIDs.includes(project.id)} onChange={() => setProjectIDs((items) => items.includes(project.id) ? items.filter((id) => id !== project.id) : [...items, project.id])} />
                <span>{project.title}</span>
              </label>
            ))}</div>}
          </fieldset>
          <div className="modalActions taskEditorActions"><button type="button" className="secondaryButton" onClick={() => setCreateOpen(false)}>Отмена</button><button className="primaryButton" disabled={!title.trim() || (!dueDate && !category.trim())}>Создать задачу</button></div>
        </form>
      </Modal>}
      </section>
    </div>
  );
}

function TaskRow({ task, currentDate, moveUpTask, moveDownTask, onEdit, onStatus, onMoveToToday, onMoveInDay, orderBusy = false, showOverdueDate = false, boardCard = false, onDelete }: {
  task: Task;
  currentDate: string;
  moveUpTask?: Task;
  moveDownTask?: Task;
  onEdit: (task: Task) => void;
  onStatus: (task: Task, status: TaskStatus) => Promise<void>;
  onMoveToToday: (task: Task, date: string) => Promise<void>;
  onMoveInDay?: (task: Task, otherTask: Task) => Promise<void>;
  orderBusy?: boolean;
  showOverdueDate?: boolean;
  boardCard?: boolean;
  onDelete: (task: Task) => Promise<void>;
}) {
  const [actionsOpen, setActionsOpen] = useState(false);
  const [completing, setCompleting] = useState(false);
  const completionTimer = useRef<number | null>(null);
  const creating = task.id < 0;
  const overdue = task.status === "todo" && Boolean(task.dueDate) && task.dueDate < currentDate;
  const visibleCategory = task.category.trim();

  useEffect(() => () => {
    if (completionTimer.current !== null) window.clearTimeout(completionTimer.current);
  }, []);

  function clearPendingCompletion() {
    if (completionTimer.current !== null) window.clearTimeout(completionTimer.current);
    completionTimer.current = null;
    setCompleting(false);
  }

  function toggleStatusWithUndo() {
    if (task.status === "done") {
      void onStatus(task, "todo");
      return;
    }
    if (completing) {
      clearPendingCompletion();
      return;
    }
    setActionsOpen(false);
    setCompleting(true);
    completionTimer.current = window.setTimeout(() => {
      completionTimer.current = null;
      void onStatus(task, "done").finally(() => setCompleting(false));
    }, 1500);
  }

  return <article className={`taskRow task-${task.status} taskPriority-${task.priority} ${boardCard ? "taskBoardCard" : ""} ${actionsOpen ? "taskActionsOpen" : ""} ${completing ? "taskCompleting" : ""}`}>
    <button className="taskCheck" disabled={creating} onClick={toggleStatusWithUndo} aria-label={`${creating ? "Задача сохраняется" : completing ? "Отменить выполнение задачи" : task.status === "done" ? "Вернуть задачу" : "Выполнить задачу"}; приоритет ${task.priority}`} title={creating ? "Задача сохраняется" : completing ? "Нажмите ещё раз, чтобы отменить" : undefined}>{task.status === "done" || completing ? "✓" : ""}</button>
    <button className="taskBody" onClick={() => onEdit(task)} disabled={creating || completing} title={task.description || task.title}>
      <span className="taskName">{task.title}</span>
      {boardCard && !completing && <span className="taskCardMeta">
        <time className={`taskCardDate${overdue ? " taskCardDateOverdue" : ""}`} dateTime={task.dueDate ? `${task.dueDate}${task.dueTime ? `T${task.dueTime}` : ""}` : undefined}>{task.dueDate ? formatTaskDueDate(task.dueDate, currentDate) : "Без даты"}{task.dueTime ? ` · ${task.dueTime}` : ""}</time>
        {task.category.trim() && <span className="taskCardContext">{task.category.trim()}</span>}
        {task.reminderAt && !task.reminderSentAt && <span className="taskReminderMark" title={`Напоминание: ${new Date(task.reminderAt).toLocaleString("ru-RU")}`}>◷</span>}
      </span>}
      {boardCard && completing && <span className="taskCardMeta"><span className="taskUndoHint">Ещё раз — отменить</span></span>}
    </button>
    {!boardCard && <div className="taskRowMeta">
      {completing ? <span className="taskUndoHint">Ещё раз — отменить</span> : <>
        {visibleCategory && <span className="taskCategory">{visibleCategory}</span>}
        {task.reminderAt && !task.reminderSentAt && <span className="taskReminderMark" title={`Напоминание: ${new Date(task.reminderAt).toLocaleString("ru-RU")}`}>◷</span>}
        {overdue && showOverdueDate ? (
          <time className="taskDueDate taskDueDateOverdue" dateTime={task.dueDate}>{formatOverdueTaskDate(task.dueDate, currentDate)}</time>
        ) : task.status === "todo" && task.dueTime ? (
          <time className="taskDueDate" dateTime={task.dueTime}>{task.dueTime}</time>
        ) : null}
      </>}
    </div>}
    <button className="taskMore" disabled={creating || completing} onClick={() => setActionsOpen((open) => !open)} aria-label="Действия с задачей" aria-expanded={actionsOpen}>⋯</button>
    {actionsOpen && <div className="taskActions">
      {task.status === "todo" && onMoveInDay && <div className="taskOrderActions" aria-label="Порядок задачи в списке">
        <button type="button" disabled={orderBusy || !moveUpTask} onClick={() => { if (!moveUpTask || orderBusy) return; void onMoveInDay(task, moveUpTask); }} aria-label="Переместить задачу выше"><OrderArrowIcon direction="up" /></button>
        <button type="button" disabled={orderBusy || !moveDownTask} onClick={() => { if (!moveDownTask || orderBusy) return; void onMoveInDay(task, moveDownTask); }} aria-label="Переместить задачу ниже"><OrderArrowIcon direction="down" /></button>
      </div>}
      {task.status === "done" && <button onClick={() => void onStatus(task, "todo")}>Вернуть</button>}
      {overdue && <button onClick={() => { setActionsOpen(false); void onMoveToToday(task, currentDate); }}>Перенести на сегодня</button>}
      <button onClick={() => { setActionsOpen(false); onEdit(task); }}>Изменить</button>
      <button className="taskDelete" onClick={() => { setActionsOpen(false); void onDelete(task); }}>Удалить</button>
    </div>}
  </article>;
}

function OrderArrowIcon({ direction }: { direction: "up" | "down" }) {
  return <svg className="orderArrowIcon" viewBox="0 0 24 24" aria-hidden="true"><path d={direction === "up" ? "M12 19V5M6.5 10.5 12 5l5.5 5.5" : "M12 5v14m-5.5-5.5L12 19l5.5-5.5"} /></svg>;
}


function FilterIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M4 7h10M18 7h2M4 17h2M10 17h10M14 4v6M7 14v6" />
    </svg>
  );
}

function AllTasksIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M4 18l5.2-5.4 3.5 3.2L20 7.5" />
      <path d="M14.8 7.5H20v5.2" />
    </svg>
  );
}

function MonthCalendar({
  monthDate,
  selectedDate,
  currentDate,
  tasks,
  onMove,
  onSelect,
  onToday,
}: {
  monthDate: string;
  selectedDate: string | null;
  currentDate: string;
  tasks: Task[];
  onMove: (direction: -1 | 1) => void;
  onSelect: (date: string) => void;
  onToday: () => void;
}) {
  const parsedMonth = parseDateKey(monthDate) ?? new Date();
  const monthIndex = parsedMonth.getMonth();
  const gridStart = startOfWeekDateKey(startOfMonthDateKey(monthDate));
  const days = Array.from({ length: 42 }, (_, index) => {
    const date = addDaysToDateKey(gridStart, index);
    const parsed = parseDateKey(date);
    return {
      date,
      day: parsed?.getDate() ?? index + 1,
      outside: parsed?.getMonth() !== monthIndex,
      taskCount: tasks.filter((task) => task.dueDate === date && task.status !== "done").length,
    };
  });
  return (
    <div className="taskMonthCalendar">
      <header className="taskMonthHeader">
        <button type="button" onClick={() => onMove(-1)} aria-label="Предыдущий месяц"><span aria-hidden="true">‹</span></button>
        <strong aria-live="polite">{formatMonthYear(monthDate)}</strong>
        <button type="button" onClick={() => onMove(1)} aria-label="Следующий месяц"><span aria-hidden="true">›</span></button>
      </header>
      <div className="taskMonthWeekdays" aria-hidden="true">
        {['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс'].map((day) => <span key={day}>{day}</span>)}
      </div>
      <div className="taskMonthGrid" role="group" aria-label={formatMonthYear(monthDate)}>
        {days.map((day) => {
          const selected = day.date === selectedDate;
          const isToday = day.date === currentDate;
          return (
            <button
              key={day.date}
              type="button"
              className={`taskMonthDay ${day.outside ? "taskMonthDayOutside" : ""} ${selected ? "taskMonthDaySelected" : ""} ${isToday ? "taskMonthDayToday" : ""}`}
              onClick={() => onSelect(day.date)}
              aria-label={`${formatSelectedDate(day.date)}${day.taskCount ? `, активных задач: ${day.taskCount}` : ""}`}
              aria-pressed={selected}
            >
              <span>{day.day}</span>
              <i className={day.taskCount ? "hasTasks" : ""} aria-hidden="true" />
            </button>
          );
        })}
      </div>
      <button className="taskMonthToday" type="button" onClick={onToday}>Сегодня</button>
    </div>
  );
}

export function ProjectsPage({
  goals,
  tasks,
  onNew,
  onEdit,
  onComplete,
  onReopen,
  onMove,
  onStep,
  orderBusy,
}: {
  goals: Goal[];
  tasks: Task[];
  onNew: () => void;
  onEdit: (goal: Goal) => void;
  onComplete: (goal: Goal) => Promise<void>;
  onReopen: (goal: Goal) => Promise<void>;
  onMove: (goal: Goal, direction: -1 | 1) => Promise<void>;
  onStep: (goal: Goal, direction: -1 | 1) => Promise<void>;
  orderBusy: boolean;
}) {
  const taskMap = new Map(tasks.map((task) => [String(task.id), task]));
  const active = goals.filter((goal) => !goal.completed);
  const completed = goals.filter((goal) => goal.completed);
  return (
    <section className="projectsSection">
      <header className="projectsHero">
        <div>
          <div className="eyebrow">Проекты · основа портфолио</div>
          <h1>Крупные результаты — отдельно от рутины.</h1>
          <p>Завершённые проекты сохраняются в портфолио и остаются в общей хронологии результатов.</p>
        </div>
        <button className="primaryButton projectNew" onClick={onNew}>＋ Новый проект <kbd>N</kbd></button>
      </header>

      <div className="projectCounters">
        <span><b>{active.length}</b> активных</span>
        <span><b>{completed.length}</b> завершённых</span>
      </div>

      {goals.length === 0 ? (
        <EmptyState title="Проектов пока нет" text="Создайте крупную веху, ведите прогресс и сохраните результат в портфолио." action="Создать проект" onAction={onNew} />
      ) : (
        <div className="projectList">
          {goals.map((goal) => {
            const related = goal.relatedTaskIds.map((id) => taskMap.get(id)).filter(Boolean) as Task[];
            return (
              <article className={`projectCard ${goal.completed ? "projectCompleted" : ""}`} key={goal.id}>
                <div className="projectTop">
                  <div>
                    <div className="projectBadges">
                      <span>{goal.completed ? "ЗАВЕРШЁН" : "АКТИВЕН"}</span>
                      {goal.pinned && <span className="stampBadge">НА КАРТЕ</span>}
                      {goal.deadline && <span>ДО {goal.deadline}</span>}
                    </div>
                    <h2>{goal.title}</h2>
                    {(goal.description || goal.summary) && <p>{goal.description || goal.summary}</p>}
                  </div>
                  <div className="projectCardAside">
                    <div className="projectOrderControls" aria-label={`Порядок проекта ${goal.title}`}>
                      <button type="button" disabled={orderBusy || goals[0]?.id === goal.id} onClick={() => void onMove(goal, -1)} aria-label="Переместить проект выше"><OrderArrowIcon direction="up" /></button>
                      <button type="button" disabled={orderBusy || goals[goals.length - 1]?.id === goal.id} onClick={() => void onMove(goal, 1)} aria-label="Переместить проект ниже"><OrderArrowIcon direction="down" /></button>
                    </div>
                    <strong className="projectPercent">{goal.completionPct}%</strong>
                  </div>
                </div>
                <div className="projectProgress"><div style={{ width: `${goal.completionPct}%` }} /></div>
                <div className="projectNumbers">{formatValue(goal.currentValue)} / {formatValue(goal.targetValue)} {goal.unit}</div>
                {related.length > 0 && (
                  <div className="relatedTasks">Связано: {related.map((task) => task.title).join(" · ")}</div>
                )}
                {goal.completed && <div className="projectSummary">{goal.summary || "Завершённый проект"}</div>}
                <div className="projectActions">
                  <button className="textButton" onClick={() => onEdit(goal)}>Изменить</button>
                  {!goal.completed && <div className="projectStageControls" aria-label={`Этапы проекта ${goal.title}`}>
                    <button type="button" disabled={goal.currentValue <= 0} onClick={() => void onStep(goal, -1)}>−1</button>
                    <button type="button" disabled={goal.currentValue >= goal.targetValue} onClick={() => void onStep(goal, 1)}>＋1</button>
                  </div>}
                  {goal.completed ? (
                    <button className="textButton" onClick={() => void onReopen(goal)}>Вернуть в работу</button>
                  ) : (
                    <button className="completeProject" onClick={() => void onComplete(goal)}>Завершить проект</button>
                  )}
                </div>
              </article>
            );
          })}
        </div>
      )}
      <button type="button" className="createFab" onClick={onNew}><span aria-hidden="true">＋</span> Проект</button>
    </section>
  );
}

function ProfilePage({
  user,
  themePreferences,
  onThemeChange,
  profile,
  onProfileChanged,
  workspace,
  onWorkspaceSettings,
  trackersEnabled,
  bottomNavigation,
  bottomNavigationSaving,
  onOpenCard,
  onTrackersEnabledChange,
  onBottomNavigationChange,
  onLogout,
  onReset,
}: {
  user: AuthUser;
  themePreferences: ThemePreferences;
  onThemeChange: (preferences: ThemePreferences) => void;
  profile: Profile;
  onProfileChanged: () => Promise<void>;
  workspace: WorkspacePreferences | null;
  onWorkspaceSettings: () => void;
  trackersEnabled: boolean;
  bottomNavigation: BottomNavigationItem[];
  bottomNavigationSaving: boolean;
  onOpenCard: () => void;
  onTrackersEnabledChange: (enabled: boolean) => void;
  onBottomNavigationChange: (items: BottomNavigationItem[]) => Promise<void>;
  onLogout: () => Promise<void>;
  onReset: () => Promise<boolean>;
}) {
  const [fatSecretStatus, setFatSecretStatus] = useState<FatSecretStatus | null>(null);
  const [fatSecretBusy, setFatSecretBusy] = useState(false);
  const [fatSecretError, setFatSecretError] = useState<string | null>(null);
  const [fatSecretNotice] = useState<FatSecretNotice | null>(consumeFatSecretCallbackNotice);
  const [logoutBusy, setLogoutBusy] = useState(false);
  const [resetConfirmOpen, setResetConfirmOpen] = useState(false);
  const [resetConfirmation, setResetConfirmation] = useState("");
  const [resetBusy, setResetBusy] = useState(false);
  const [projectProfileOpen, setProjectProfileOpen] = useState(false);

  function changeBottomNavigationSlot(index: number, nextItem: BottomNavigationItem) {
    if (bottomNavigationSaving || bottomNavigation[index] === nextItem) return;
    const next = [...bottomNavigation];
    const existingIndex = next.indexOf(nextItem);
    if (existingIndex >= 0) {
      [next[index], next[existingIndex]] = [next[existingIndex], next[index]];
    } else {
      next[index] = nextItem;
    }
    void onBottomNavigationChange(next);
  }

  useEffect(() => {
    let cancelled = false;
    api.fatSecretStatus()
      .then((status) => {
        if (!cancelled) setFatSecretStatus(status);
      })
      .catch((cause: unknown) => {
        if (!cancelled) setFatSecretError(cause instanceof Error ? cause.message : "Не удалось проверить FatSecret");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function connectFatSecret() {
    const returnURL = new URL(window.location.href);
    returnURL.searchParams.delete("fatsecret");
    returnURL.searchParams.set("view", "profile");
    const returnTo = `${returnURL.pathname}${returnURL.search}${returnURL.hash}`;
    setFatSecretBusy(true);
    setFatSecretError(null);
    try {
      const { authorizeUrl } = await api.connectFatSecret(returnTo);
      window.location.assign(authorizeUrl);
    } catch (cause) {
      setFatSecretError(cause instanceof Error ? cause.message : "Не удалось начать подключение FatSecret");
      setFatSecretBusy(false);
    }
  }

  async function disconnectFatSecret() {
    if (!fatSecretStatus?.connected || fatSecretBusy) return;
    if (!window.confirm("Отключить FatSecret? Данные дневника останутся в аккаунте FatSecret.")) return;
    setFatSecretBusy(true);
    setFatSecretError(null);
    try {
      await api.disconnectFatSecret();
      setFatSecretStatus({ configured: fatSecretStatus.configured, connected: false, connectedAt: "" });
    } catch (cause) {
      setFatSecretError(cause instanceof Error ? cause.message : "Не удалось отключить FatSecret");
    } finally {
      setFatSecretBusy(false);
    }
  }

  async function logout() {
    if (logoutBusy) return;
    setLogoutBusy(true);
    try {
      await onLogout();
    } finally {
      setLogoutBusy(false);
    }
  }

  function openResetConfirmation() {
    setResetConfirmation("");
    setResetConfirmOpen(true);
  }

  function closeResetConfirmation() {
    if (resetBusy) return;
    setResetConfirmation("");
    setResetConfirmOpen(false);
  }

  async function confirmReset() {
    if (resetBusy || resetConfirmation.trim().toLocaleUpperCase("ru-RU") !== "СБРОСИТЬ") return;
    setResetBusy(true);
    try {
      const reset = await onReset();
      if (reset) {
        setResetConfirmation("");
        setResetConfirmOpen(false);
      }
    } finally {
      setResetBusy(false);
    }
  }

  const fullName = `${profile.name} ${profile.surname}`.trim() || user.login;

  return (
    <section className="profilePage" aria-labelledby="profile-page-title">
      <header className="profileHero">
        <div className="profilePortrait" aria-hidden={!profile.photo}>
          {profile.photo ? <img src={profile.photo} alt="Фото профиля" /> : <span>{fullName.slice(0, 1).toUpperCase()}</span>}
        </div>
        <div className="profileHeroText">
          <span className="profileKicker">Личный кабинет</span>
          <h1 id="profile-page-title">{fullName}</h1>
          <p>{profile.occupation || "Род занятий не указан"}</p>
        </div>
        <button type="button" className="secondaryButton profileCardLink" onClick={onOpenCard}>Открыть карту</button>
      </header>

      <section className="profileFacts" aria-label="Краткая информация о профиле">
        <div><span>Логин</span><strong>{user.login}</strong></div>
        <div><span>Дата рождения</span><strong>{profile.dob ? formatDate(profile.dob) : "Не указана"}</strong></div>
        <div><span>Пол</span><strong>{profile.sex || "Не указан"}</strong></div>
        <div><span>Карта действует до</span><strong>{CARD_EXPIRY_DATE}</strong></div>
      </section>

      <section className="profilePreferences" aria-labelledby="profile-preferences-title">
        <header>
          <span className="profileKicker">Интерфейс</span>
          <h2 id="profile-preferences-title">Настройки</h2>
        </header>
        <button type="button" className="secondaryButton" onClick={onWorkspaceSettings}>Мои инструменты</button>
        <button type="button" className="projectProfileSettingsCard" onClick={()=>setProjectProfileOpen(true)}>
          <span className="projectProfileSettingsAvatar">{profile.workShowAvatar && (profile.workAvatar || profile.photo) ? <img src={profile.workAvatar || profile.photo} alt=""/> : (profile.workDisplayName || user.login).slice(0,2).toUpperCase()}</span>
          <span><strong>Профиль в проектах</strong><small>{profile.workDisplayName || user.login} · {profile.workShowAvatar ? "Фото показывается" : "Фото скрыто"}</small></span>
          <b aria-hidden="true">›</b>
        </button>
        <ThemeSettings preferences={themePreferences} onChange={onThemeChange} />
        <div className="bottomNavigationHints">
          <p><strong>Калории</strong> остаются доступны через карточку «Калории» в трекерах на карте профиля и из раздела «Трекер», даже если вкладка скрыта.</p>
          <p><strong>Проекты</strong> остаются доступны через блок «Портфолио» на карте профиля, даже если вкладка скрыта.</p>
        </div>
        {workspace?.features.includes("widgets") && <label className="profileToggle">
          <span><strong>Показывать трекеры</strong><small>Область с трекерами под картой профиля</small></span>
          <input type="checkbox" checked={trackersEnabled} onChange={(event) => onTrackersEnabledChange(event.target.checked)} />
          <i aria-hidden="true" />
        </label>}
        {!workspace?.onboardingCompleted && <div className="bottomNavigationSettings">
          <div className="bottomNavigationSettingsHead">
            <span><strong>Нижняя панель</strong><small>Карта, Задачи и Профиль обязательны. Четвёртый раздел можно выбрать и поставить в любую позицию.</small></span>
            <button
              type="button"
              className="textButton"
              disabled={bottomNavigationSaving || bottomNavigation.every((item, index) => item === DEFAULT_BOTTOM_NAVIGATION[index])}
              onClick={() => void onBottomNavigationChange([...DEFAULT_BOTTOM_NAVIGATION])}
            >По умолчанию</button>
          </div>
          <div className="bottomNavigationSlots" aria-label="Разделы нижней панели">
            {bottomNavigation.map((item, index) => (
              <label className="bottomNavigationSlot" key={index}>
                <span>{index + 1}</span>
                <select
                  value={item}
                  disabled={bottomNavigationSaving}
                  aria-label={`Раздел нижней панели, позиция ${index + 1}`}
                  onChange={(event) => changeBottomNavigationSlot(index, event.target.value as BottomNavigationItem)}
                >
                  {BOTTOM_NAVIGATION_OPTIONS.map((option) => (
                    <option
                      value={option.id}
                      key={option.id}
                      disabled={REQUIRED_BOTTOM_NAVIGATION.has(item) && !bottomNavigation.includes(option.id)}
                    >{option.label}{REQUIRED_BOTTOM_NAVIGATION.has(option.id) ? " · обязательно" : ""}</option>
                  ))}
                </select>
              </label>
            ))}
          </div>
        </div>}
      </section>
      {projectProfileOpen && <Suspense fallback={null}><ProjectProfileDialog login={user.login} profile={profile} onClose={()=>setProjectProfileOpen(false)} onSaved={onProfileChanged}/></Suspense>}

      <section className="profileIntegrations" aria-labelledby="profile-integrations-title">
        <header>
          <span className="profileKicker">Подключения</span>
          <h2 id="profile-integrations-title">Интеграции</h2>
        </header>

        <article className="profileIntegrationCard">
          <div className="profileIntegrationHead">
            <div>
              <span className="integrationMonogram">FS</span>
              <div><strong>FatSecret</strong><small>Калории и пищевой дневник</small></div>
            </div>
            <span className={`profileIntegrationStatus ${fatSecretStatus?.connected ? "isConnected" : ""}`}>
              {fatSecretStatus === null ? "Проверка" : fatSecretStatus.connected ? "Подключено" : "Не подключено"}
            </span>
          </div>
          {fatSecretNotice && <div className={`integrationNotice integrationNotice-${fatSecretNotice.tone}`}>{fatSecretNotice.text}</div>}
          {fatSecretError && <div className="integrationNotice integrationNotice-error">{fatSecretError}</div>}
          <p className="integrationExplanationDefault">Подключается только существующий аккаунт. Логин и пароль FatSecret не передаются identity workspace.</p>
          <details className="integrationExplanationDesktop"><summary>Подробнее</summary><p>Подключается только существующий аккаунт. Логин и пароль FatSecret не передаются identity workspace.</p></details>
          {!fatSecretStatus ? null : !fatSecretStatus.configured ? (
            <p className="integrationHint">Добавьте Consumer Key и Consumer Secret FatSecret в серверный <code>.env</code>.</p>
          ) : fatSecretStatus.connected ? (
            <button type="button" className="secondaryButton" disabled={fatSecretBusy} onClick={() => void disconnectFatSecret()}>{fatSecretBusy ? "ОТКЛЮЧЕНИЕ…" : "Отключить FatSecret"}</button>
          ) : (
            <button type="button" className="primaryButton" onClick={connectFatSecret}>Подключить FatSecret</button>
          )}
        </article>

      </section>

      {user.isAdmin && (
        <section className="profileAdminTools" aria-labelledby="profile-admin-tools-title">
          <div><span className="profileKicker">Администрирование</span><h2 id="profile-admin-tools-title">Панель администратора</h2><p>Модерируйте пользовательские продукты, добавляйте их в глобальную базу и просматривайте логи приложения.</p></div>
          <a href="/admin/foods">Открыть панель <span aria-hidden="true">→</span></a>
        </section>
      )}

      <section className="profileAccountActions" aria-labelledby="profile-account-actions-title">
        <header>
          <span className="profileKicker">Аккаунт</span>
          <h2 id="profile-account-actions-title">Сеанс и данные</h2>
          <p>Управление текущим сеансом и рабочими данными аккаунта.</p>
        </header>

        <div className="profileActionRow">
          <span className="profileActionCopy">
            <strong>Выйти из аккаунта</strong>
            <small>На этом устройстве потребуется войти снова.</small>
          </span>
          <button type="button" className="profileLogoutButton" disabled={logoutBusy} onClick={() => void logout()}>{logoutBusy ? "Выход…" : "Выйти"}</button>
        </div>

        <div className={`profileDangerZone${resetConfirmOpen ? " isOpen" : ""}`}>
          <span className="profileActionCopy">
            <strong>Сбросить задачи и проекты</strong>
            <small>Профиль и фотография сохранятся, но рабочие данные будут удалены.</small>
          </span>
          {!resetConfirmOpen && <button type="button" className="profileResetReveal" onClick={openResetConfirmation}>Сбросить</button>}

          {resetConfirmOpen && (
            <div className="profileResetConfirm">
              <p>Это действие нельзя отменить. Для продолжения введите <strong>СБРОСИТЬ</strong>.</p>
              <label>
                <span>Подтверждение</span>
                <input
                  value={resetConfirmation}
                  autoComplete="off"
                  autoCapitalize="characters"
                  spellCheck={false}
                  disabled={resetBusy}
                  placeholder="СБРОСИТЬ"
                  onChange={(event) => setResetConfirmation(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key !== "Enter") return;
                    event.preventDefault();
                    void confirmReset();
                  }}
                />
              </label>
              <div className="profileResetActions">
                <button type="button" className="profileResetCancel" disabled={resetBusy} onClick={closeResetConfirmation}>Отмена</button>
                <button
                  type="button"
                  className="profileResetExecute"
                  disabled={resetBusy || resetConfirmation.trim().toLocaleUpperCase("ru-RU") !== "СБРОСИТЬ"}
                  onClick={() => void confirmReset()}
                >
                  {resetBusy ? "Удаление…" : "Удалить задачи и проекты"}
                </button>
              </div>
            </div>
          )}
        </div>
      </section>
    </section>
  );
}

function TaskEditor({
  task,
  currentDate,
  onClose,
  onSave,
}: {
  task: Task;
  currentDate: string;
  onClose: () => void;
  onSave: (input: TaskInput) => Promise<void>;
}) {
  const [title, setTitle] = useState(task.title);
  const [description, setDescription] = useState(task.description);
  const [category, setCategory] = useState(task.category);
  const [status, setStatus] = useState<TaskStatus>(task.status);
  const [dueDate, setDueDate] = useState(task.dueDate);
  const [dueTime, setDueTime] = useState(task.dueTime);
  const [reminderLocal, setReminderLocal] = useState(reminderToInput(task.reminderAt));
  const [priority, setPriority] = useState(task.priority);
  const [recurrenceType, setRecurrenceType] = useState<TaskRecurrenceType>(task.recurrenceType || "");
  const [recurrenceInterval, setRecurrenceInterval] = useState(task.recurrenceInterval || 1);
  const [recurrenceEndDate, setRecurrenceEndDate] = useState(task.recurrenceEndDate || "");
  const [recurrenceWeekdays, setRecurrenceWeekdays] = useState<number[]>(task.recurrenceWeekdays || []);
  const [categories, setCategories] = useState<TaskCategory[]>([]);
  const keyboardOpen = useVirtualKeyboardOpen();
  const [notificationState, setNotificationState] = useState<DeviceNotificationState>(() => {
    if (!("Notification" in window) || !("serviceWorker" in navigator) || !("PushManager" in window)) return "unsupported";
    return Notification.permission === "granted" ? "enabled" : Notification.permission === "denied" ? "denied" : "unknown";
  });
  const categoryAllowsUndated = Boolean(category.trim());
  const scheduleIsValid = Boolean(dueDate) || categoryAllowsUndated;

  useEffect(() => {
    api.taskCategories().then(setCategories).catch(() => undefined);
  }, []);

  async function createEditorCategory(name: string) {
    const saved = await api.createTaskCategory(name);
    setCategories((items) => [...items.filter((item) => item.id !== saved.id), saved]);
    return saved.name;
  }

  const editorCategories = useMemo(() => {
    const items = [...categories];
    if (task.category && !items.some((item) => item.name.localeCompare(task.category, "ru", { sensitivity: "accent" }) === 0)) {
      items.push({ id: 0, name: task.category, builtin: true });
    }
    return items;
  }, [categories, task.category]);

  return (
    <Modal onClose={onClose} title="Редактировать задачу" wide>
      <form className={`taskEditorForm ${keyboardOpen ? "taskEditorKeyboardOpen" : ""}`} noValidate onSubmit={(event) => {
        event.preventDefault();
        if (!title.trim() || !scheduleIsValid) return;
        void onSave({
          title: title.trim(),
          description: description.trim(),
          category: category.trim(),
          status,
          dueDate,
          dueTime: dueDate ? dueTime : "",
          reminderAt: reminderToRFC3339(reminderLocal),
          priority,
          isMilestone: priority === 3,
          recurrenceType,
          recurrenceInterval,
          recurrenceEndDate,
          recurrenceWeekdays,
        });
      }}>
        <Field label="Название"><input className="input" value={title} maxLength={160} onChange={(event) => setTitle(event.target.value)} /></Field>
        <div className="field taskDescriptionField"><span className="fieldLabel">Описание</span><TaskDescriptionEditor taskID={task.id} value={description} onChange={setDescription} /></div>
        <div className="taskFormGrid">
          <div className="field"><span className="fieldLabel">Категория</span><TaskCategoryPicker value={category} categories={editorCategories} onChange={setCategory} onCreate={createEditorCategory} /></div>
          <div className="field"><span className="fieldLabel">Дата</span><TaskDateControl value={dueDate} currentDate={currentDate} allowUndated={categoryAllowsUndated} onChange={(value) => { setDueDate(value); if (!value) setDueTime(""); }} /></div>
          <div className="field"><span className="fieldLabel">Время задачи</span><TaskScheduleInput type="time" value={dueTime} disabled={!dueDate} onChange={setDueTime} /></div>
          <div className="field"><span className="fieldLabel">Напомнить</span><TaskScheduleInput type="datetime-local" value={reminderLocal} onChange={setReminderLocal} /></div>
        </div>
        <TaskRecurrenceControl dueDate={dueDate} type={recurrenceType} interval={recurrenceInterval} endDate={recurrenceEndDate} weekdays={recurrenceWeekdays} onType={setRecurrenceType} onInterval={setRecurrenceInterval} onEndDate={setRecurrenceEndDate} onWeekdays={setRecurrenceWeekdays} />
        <Field label="Статус"><select className="input" value={status} onChange={(event) => setStatus(event.target.value as TaskStatus)}><option value="todo">Не выполнена</option><option value="done">Выполнена</option></select></Field>
        <div className="field"><span className="fieldLabel">Приоритет</span><TaskPriorityPicker value={priority} onChange={setPriority} /></div>
        {reminderLocal && notificationState !== "enabled" && <div className="notificationSetup"><div><strong>Уведомления на устройство</strong><span>{notificationState === "denied" ? "Разрешение заблокировано в настройках браузера." : notificationState === "unsupported" ? "Этот браузер или режим не поддерживает Web Push." : "Разрешите уведомления, чтобы напоминание пришло даже при закрытом приложении. На iPhone установите PWA на экран «Домой»."}</span></div>{notificationState !== "denied" && notificationState !== "unsupported" && <button type="button" className="secondaryButton" onClick={() => void registerDeviceNotifications(true).then(setNotificationState)}>Включить</button>}</div>}
        <div className="modalActions taskEditorActions"><button type="button" className="secondaryButton" onClick={onClose}>Отмена</button><button className="primaryButton" disabled={!title.trim() || !scheduleIsValid}>Сохранить</button></div>
      </form>
    </Modal>
  );
}

function TaskRecurrenceControl({
  dueDate,
  type,
  interval,
  endDate,
  weekdays,
  onType,
  onInterval,
  onEndDate,
  onWeekdays,
}: {
  dueDate: string;
  type: TaskRecurrenceType;
  interval: number;
  endDate: string;
  weekdays: number[];
  onType: (value: TaskRecurrenceType) => void;
  onInterval: (value: number) => void;
  onEndDate: (value: string) => void;
  onWeekdays: (value: number[]) => void;
}) {
  const [open, setOpen] = useState(false);
  const labels: Record<Exclude<TaskRecurrenceType, "">, string> = {
    daily: "Каждый день",
    weekly: "Каждую неделю",
    monthly: "Каждый месяц",
  };
  const weekdayLabels = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];
  const weeklySummary = weekdays.length > 0
    ? `По ${weekdays.map((weekday) => weekdayLabels[weekday - 1].toLocaleLowerCase("ru-RU")).join(", ")}`
    : labels.weekly;
  const summary = type
    ? interval === 1
      ? type === "weekly" ? weeklySummary : labels[type]
      : `${type === "weekly" ? weeklySummary : labels[type]} · интервал ${interval}`
    : "Повторять";

  useEffect(() => {
    if (!dueDate && type) {
      onType("");
      onInterval(1);
      onEndDate("");
      onWeekdays([]);
      return;
    }
    if (dueDate && endDate && endDate < dueDate) onEndDate("");
  }, [dueDate, endDate, type, onType, onInterval, onEndDate, onWeekdays]);

  function choose(next: Exclude<TaskRecurrenceType, "">) {
    onType(next);
    if (interval < 1) onInterval(1);
    if (next !== "weekly") {
      onWeekdays([]);
    } else if (weekdays.length === 0) {
      const date = parseDateKey(dueDate);
      if (date) onWeekdays([date.getDay() === 0 ? 7 : date.getDay()]);
    }
  }

  function toggleWeekday(weekday: number) {
    if (weekdays.includes(weekday)) {
      if (weekdays.length === 1) return;
      onWeekdays(weekdays.filter((value) => value !== weekday));
      return;
    }
    onWeekdays([...weekdays, weekday].sort((left, right) => left - right));
  }

  return (
    <div className={`taskRecurrence ${open ? "isOpen" : ""}`}>
      <button type="button" className="taskRecurrenceToggle" onClick={() => setOpen((value) => !value)} aria-expanded={open}>
        <span aria-hidden="true">↻</span><strong>{summary}</strong><b aria-hidden="true">⌄</b>
      </button>
      {open && <div className="taskRecurrencePanel">
        {!dueDate && <p>Сначала выберите дату задачи.</p>}
        <div className="taskRecurrenceChoices" role="group" aria-label="Частота повторения">
          {(["daily", "weekly", "monthly"] as const).map((value) => <button type="button" key={value} className={type === value ? "active" : ""} disabled={!dueDate} aria-pressed={type === value} onClick={() => choose(value)}>{labels[value]}</button>)}
        </div>
        {type === "weekly" && <div className="taskRecurrenceWeekdays">
          <span>Дни недели</span>
          <div role="group" aria-label="Дни недели для повторения">
            {weekdayLabels.map((label, index) => {
              const weekday = index + 1;
              const selected = weekdays.includes(weekday);
              return <button type="button" key={weekday} className={selected ? "active" : ""} aria-pressed={selected} onClick={() => toggleWeekday(weekday)}>{label}</button>;
            })}
          </div>
        </div>}
        {type && <div className="taskRecurrenceDetails">
          <label><span>Интервал, {type === "daily" ? "дней" : type === "weekly" ? "недель" : "месяцев"}</span><input className="input" type="number" inputMode="numeric" min={1} max={365} value={interval} onChange={(event) => onInterval(Math.min(365, Math.max(1, Number(event.target.value) || 1)))} /></label>
          <label><span>Повторять до</span><input
            className="input"
            type="date"
            min={dueDate}
            value={endDate}
            onInput={(event) => onEndDate(event.currentTarget.value)}
            onChange={(event) => onEndDate(event.currentTarget.value)}
          /></label>
          <button type="button" className="taskRecurrenceClear" onClick={() => { onType(""); onInterval(1); onEndDate(""); onWeekdays([]); }}>Не повторять</button>
        </div>}
      </div>}
    </div>
  );
}

function TaskDateControl({
  value,
  currentDate,
  allowUndated,
  onChange,
}: {
  value: string;
  currentDate: string;
  allowUndated: boolean;
  onChange: (value: string) => void;
}) {
  const tomorrow = addDaysToDateKey(currentDate, 1);
  return (
    <div className="taskDateControl">
      <input className="input taskDateInput" type="date" value={value} onInput={(event) => onChange(event.currentTarget.value)} onChange={(event) => onChange(event.currentTarget.value)} />
      <div className="taskDateQuick" role="group" aria-label="Быстрый выбор даты">
        <button type="button" className={value === currentDate ? "active" : ""} aria-pressed={value === currentDate} onClick={() => onChange(currentDate)}>Сегодня</button>
        <button type="button" className={value === tomorrow ? "active" : ""} aria-pressed={value === tomorrow} onClick={() => onChange(tomorrow)}>Завтра</button>
        <button type="button" className={!value ? "active" : ""} aria-pressed={!value} disabled={!allowUndated} title={!allowUndated ? "Сначала укажите категорию" : undefined} onClick={() => onChange("")}>Без даты</button>
      </div>
      <small className={`taskDateRule ${allowUndated ? "" : "taskDateRuleRequired"}`}>Без даты — только при заполненной категории.</small>
    </div>
  );
}

function GoalEditor({
  goal,
  tasks,
  onClose,
  onSave,
  onDelete,
}: {
  goal: Goal | null;
  tasks: Task[];
  onClose: () => void;
  onSave: (input: GoalInput) => Promise<void>;
  onDelete: (goal: Goal) => Promise<void>;
}) {
  const [title, setTitle] = useState(goal?.title ?? "");
  const [description, setDescription] = useState(goal?.description ?? "");
  const [summary, setSummary] = useState(goal?.summary ?? "");
  const [current, setCurrent] = useState(String(goal?.currentValue ?? 0));
  const [target, setTarget] = useState(String(goal?.targetValue ?? 1));
  const [unit, setUnit] = useState(goal?.unit ?? "");
  const deadlineRef = useRef<HTMLInputElement>(null);
  const [related, setRelated] = useState<string[]>(goal?.relatedTaskIds.filter((id) => tasks.some((task) => String(task.id) === id)) ?? []);
  const [completed, setCompleted] = useState(goal?.completed ?? false);
  const [localError, setLocalError] = useState("");

  function submit(event: React.FormEvent) {
    event.preventDefault();
    const currentValue = Number(current);
    const targetValue = Number(target);
    const deadline = deadlineRef.current?.value ?? "";
    if (!title.trim()) return setLocalError("Укажите название проекта.");
    if (!Number.isFinite(currentValue) || currentValue < 0 || !Number.isFinite(targetValue) || targetValue <= 0) return setLocalError("Проверьте значения прогресса.");
    const finalCompleted = completed || currentValue >= targetValue;
    void onSave({
      title: title.trim(), description: description.trim(), summary: summary.trim(),
      currentValue, targetValue, unit: unit.trim(), deadline,
      relatedTaskIds: related, completed: finalCompleted, pinned: finalCompleted && Boolean(goal?.pinned),
    });
  }

  return (
    <Modal onClose={onClose} title={goal ? "Редактировать проект" : "Новый проект"} wide>
      <form noValidate onSubmit={submit}>
        <Field label="Название"><input className="input" value={title} maxLength={80} autoFocus placeholder="Например: Запустить сервис аналитики" onChange={(e) => setTitle(e.target.value)} /></Field>
        <Field label="Краткое резюме для карты (одно предложение)"><input className="input" value={summary} maxLength={180} placeholder="Что именно было сделано и почему это важно" onChange={(e) => setSummary(e.target.value)} /></Field>
        <Field label="Описание"><textarea className="input textarea resize-none" value={description} maxLength={1200} placeholder="Контекст, результат, детали проекта" onChange={(e) => setDescription(e.target.value)} /></Field>
        <div className="formGrid">
          <Field label="Сейчас"><input className="input" type="number" min="0" step="any" value={current} onChange={(e) => setCurrent(e.target.value)} /></Field>
          <Field label="Цель"><input className="input" type="number" min="0.01" step="any" value={target} onChange={(e) => setTarget(e.target.value)} /></Field>
          <Field label="Единица"><input className="input" value={unit} maxLength={16} placeholder="этапов / % / модулей" onChange={(e) => setUnit(e.target.value)} /></Field>
          <Field label="Дедлайн"><input ref={deadlineRef} className="input" type="date" defaultValue={goal?.deadline ?? ""} /></Field>
        </div>

        <fieldset className="relatedPicker">
          <legend>Связанные задачи</legend>
          {tasks.length === 0 ? <p>Сначала создайте задачи, если хотите связать рабочий слой с проектом.</p> : (
            <div>{tasks.map((task) => {
              const id = String(task.id);
              return <label key={task.id} className={related.includes(id) ? "selected" : ""}><input type="checkbox" checked={related.includes(id)} onChange={() => setRelated((items) => items.includes(id) ? items.filter((item) => item !== id) : [...items, id])} /><span>{task.title}</span></label>;
            })}</div>
          )}
        </fieldset>

        <div className="completionControls">
          <label className="checkLine"><input type="checkbox" checked={completed} onChange={(e) => setCompleted(e.target.checked)} /> Проект завершён</label>
        </div>

        {localError && <div className="formError">{localError}</div>}
        <div className="modalActions">
          {goal && <button type="button" className="deleteButton" onClick={() => void onDelete(goal)}>Удалить</button>}
          <button type="button" className="secondaryButton" onClick={onClose}>Отмена</button>
          <button className="primaryButton">{goal ? "Сохранить" : "Создать проект"}</button>
        </div>
      </form>
    </Modal>
  );
}

function ProfileEditor({ profile, focusField, onClose, onSaved }: { profile: Profile; focusField: ProfileField; onClose: () => void; onSaved: () => Promise<void> }) {
  const [value, setValue] = useState({
    name: profile.name,
    surname: profile.surname,
    occupation: profile.occupation,
    sex: profile.sex,
    dob: profile.dob,
    expiry: CARD_EXPIRY_DATE,
  });
  const [localError, setLocalError] = useState("");
  const update = (key: keyof typeof value, next: string) => setValue((current) => ({ ...current, [key]: next }));
  return (
    <Modal onClose={onClose} title="Профиль">
      <form noValidate onSubmit={async (e) => {
        e.preventDefault();
        try {
          await api.updateProfile(value);
          await onSaved();
        } catch (cause) {
          setLocalError(cause instanceof Error ? cause.message : String(cause));
        }
      }}>
        <div className="formGrid">
          <Field label="Имя"><input className="input" value={value.name} maxLength={24} autoFocus={focusField === "name"} onChange={(e) => update("name", e.target.value)} /></Field>
          <Field label="Фамилия"><input className="input" value={value.surname} maxLength={28} autoFocus={focusField === "surname"} onChange={(e) => update("surname", e.target.value)} /></Field>
        </div>
        <Field label="Род занятий (ручное поле)"><input className="input" value={value.occupation} maxLength={48} autoFocus={focusField === "occupation"} onChange={(e) => update("occupation", e.target.value)} /></Field>
        <div className="formGrid">
          <Field label="Пол"><input className="input" value={value.sex} maxLength={16} autoFocus={focusField === "sex"} placeholder="М / Ж / X" onChange={(e) => update("sex", e.target.value)} /></Field>
          <Field label="Дата рождения"><input className="input" value={value.dob} maxLength={10} inputMode="numeric" autoComplete="bday" autoFocus={focusField === "dob"} placeholder="15.05.2006" onChange={(e) => update("dob", formatProfileDateInput(e.target.value))} /></Field>
        </div>
        {localError && <div className="formError">{localError}</div>}
        <div className="modalActions"><button type="button" className="secondaryButton" onClick={onClose}>Отмена</button><button className="primaryButton">Сохранить</button></div>
      </form>
    </Modal>
  );
}

function SignatureEditor({
  initialValue,
  onClose,
  onSave,
}: {
  initialValue: string;
  onClose: () => void;
  onSave: (data: string) => Promise<void>;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const activePointer = useRef<number | null>(null);
  const lastPoint = useRef<{ x: number; y: number } | null>(null);
  const changed = useRef(false);
  const [hasInk, setHasInk] = useState(Boolean(initialValue));
  const [canvasReady, setCanvasReady] = useState(!initialValue);
  const [saving, setSaving] = useState(false);
  const [localError, setLocalError] = useState("");

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const context = canvas.getContext("2d");
    if (!context) return;
    context.clearRect(0, 0, canvas.width, canvas.height);
    if (!initialValue) {
      setCanvasReady(true);
      return;
    }
    setCanvasReady(false);
    let active = true;
    const image = new Image();
    image.onload = () => {
      if (!active) return;
      if (!changed.current) {
        const scale = Math.min(canvas.width / image.naturalWidth, canvas.height / image.naturalHeight) * .9;
        const width = image.naturalWidth * scale;
        const height = image.naturalHeight * scale;
        context.drawImage(image, (canvas.width - width) / 2, (canvas.height - height) / 2, width, height);
      }
      setCanvasReady(true);
    };
    image.onerror = () => {
      if (!active) return;
      setLocalError("Не удалось открыть сохранённую подпись.");
      setCanvasReady(true);
    };
    image.src = initialValue;
    return () => { active = false; };
  }, [initialValue]);

  function canvasPoint(event: React.PointerEvent<HTMLCanvasElement>) {
    const canvas = canvasRef.current;
    if (!canvas) return null;
    const bounds = canvas.getBoundingClientRect();
    if (bounds.width === 0 || bounds.height === 0) return null;
    return {
      x: (event.clientX - bounds.left) * canvas.width / bounds.width,
      y: (event.clientY - bounds.top) * canvas.height / bounds.height,
    };
  }

  function beginStroke(event: React.PointerEvent<HTMLCanvasElement>) {
    if (activePointer.current !== null) return;
    const canvas = canvasRef.current;
    const point = canvasPoint(event);
    if (!canvas || !point) return;
    event.preventDefault();
    canvas.setPointerCapture(event.pointerId);
    activePointer.current = event.pointerId;
    lastPoint.current = point;
    changed.current = true;
    setCanvasReady(true);
    setHasInk(true);

    const context = canvas.getContext("2d");
    if (!context) return;
    context.fillStyle = "#111111";
    context.beginPath();
    context.arc(point.x, point.y, 6, 0, Math.PI * 2);
    context.fill();
  }

  function continueStroke(event: React.PointerEvent<HTMLCanvasElement>) {
    if (activePointer.current !== event.pointerId) return;
    const canvas = canvasRef.current;
    const point = canvasPoint(event);
    const previous = lastPoint.current;
    if (!canvas || !point || !previous) return;
    event.preventDefault();
    const context = canvas.getContext("2d");
    if (!context) return;
    context.strokeStyle = "#111111";
    context.lineWidth = 12;
    context.lineCap = "round";
    context.lineJoin = "round";
    context.beginPath();
    context.moveTo(previous.x, previous.y);
    context.lineTo(point.x, point.y);
    context.stroke();
    lastPoint.current = point;
  }

  function endStroke(event: React.PointerEvent<HTMLCanvasElement>) {
    if (activePointer.current !== event.pointerId) return;
    activePointer.current = null;
    lastPoint.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }

  function clearSignature() {
    const canvas = canvasRef.current;
    if (!canvas) return;
    canvas.getContext("2d")?.clearRect(0, 0, canvas.width, canvas.height);
    changed.current = true;
    setHasInk(false);
    setLocalError("");
  }

  async function saveSignature() {
    const canvas = canvasRef.current;
    if (!canvas || saving || !canvasReady || (!hasInk && !initialValue)) return;
    setSaving(true);
    setLocalError("");
    try {
      await onSave(hasInk ? croppedSignatureDataURL(canvas) : "");
    } catch (cause) {
      setLocalError(cause instanceof Error ? cause.message : String(cause));
      setSaving(false);
    }
  }

  return (
    <Modal onClose={onClose} title="Подпись">
      <div className="signatureEditor">
        <p>Нарисуйте подпись пальцем или курсором. Она появится в поле подписи на карте.</p>
        <div className="signatureCanvasFrame">
          <canvas
            ref={canvasRef}
            className="signatureCanvas"
            width="1200"
            height="360"
            role="img"
            aria-label="Область для рисования подписи"
            aria-busy={!canvasReady}
            onPointerDown={beginStroke}
            onPointerMove={continueStroke}
            onPointerUp={endStroke}
            onPointerCancel={endStroke}
          />
          {!hasInk && <span className="signatureCanvasHint">Подпишите здесь</span>}
        </div>
        <button type="button" className="signatureClear" onClick={clearSignature} disabled={!hasInk}>Очистить</button>
        {localError && <div className="formError">{localError}</div>}
        <div className="modalActions">
          <button type="button" className="secondaryButton" onClick={onClose} disabled={saving}>Отмена</button>
          <button type="button" className="primaryButton" onClick={() => void saveSignature()} disabled={saving || !canvasReady || (!hasInk && !initialValue)}>
            {saving ? "Сохранение…" : !hasInk && initialValue ? "Удалить подпись" : "Сохранить подпись"}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function croppedSignatureDataURL(canvas: HTMLCanvasElement) {
  const context = canvas.getContext("2d");
  if (!context) return canvas.toDataURL("image/png");
  const pixels = context.getImageData(0, 0, canvas.width, canvas.height).data;
  let left = canvas.width;
  let top = canvas.height;
  let right = -1;
  let bottom = -1;
  for (let y = 0; y < canvas.height; y += 1) {
    for (let x = 0; x < canvas.width; x += 1) {
      if (pixels[(y * canvas.width + x) * 4 + 3] === 0) continue;
      left = Math.min(left, x);
      top = Math.min(top, y);
      right = Math.max(right, x);
      bottom = Math.max(bottom, y);
    }
  }
  if (right < left || bottom < top) return "";
  const padding = 24;
  left = Math.max(0, left - padding);
  top = Math.max(0, top - padding);
  right = Math.min(canvas.width - 1, right + padding);
  bottom = Math.min(canvas.height - 1, bottom + padding);
  const width = right - left + 1;
  const height = bottom - top + 1;
  const output = document.createElement("canvas");
  output.width = width;
  output.height = height;
  output.getContext("2d")?.drawImage(canvas, left, top, width, height, 0, 0, width, height);
  return output.toDataURL("image/png");
}

function PortfolioTimeline({ goals, onClose }: { goals: Goal[]; onClose: () => void }) {
  return (
    <Modal onClose={onClose} title="Портфолио · хронология" wide>
      {goals.length === 0 ? (
        <EmptyState title="Нет завершённых проектов" text="После закрытия проекта здесь появится запись с датой и резюме." />
      ) : (
        <div className="timeline">
          {goals.map((goal, index) => (
            <article className="timelineItem" key={goal.id}>
              <div className="timelineRail"><span>{String(goals.length - index).padStart(2, "0")}</span></div>
              <div className="timelineContent">
                <time>{formatDate(goal.completedAt)}</time>
                <h2>{goal.title}</h2>
                <p>{goal.summary || goal.description || "Проект завершён."}</p>
                {goal.pinned && <span className="timelineStamp">НА ЛИЦЕВОЙ СТОРОНЕ</span>}
              </div>
            </article>
          ))}
        </div>
      )}
      <button className="primaryButton fullButton" onClick={onClose}>Закрыть</button>
    </Modal>
  );
}

function CaloriesWeekPicker({ value, onChange }: { value: string; onChange: (date: string) => void }) {
  const weekStart = startOfWeekDateKey(value);
  const currentWeekStart = startOfWeekDateKey(localTodayKey());
  const weekdays = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];
  const dates = weekdays.map((label, index) => ({ label, value: addDaysToDateKey(weekStart, index) }));
  const parsed = parseDateKey(value);
  const weekLabel = parsed?.toLocaleDateString("ru-RU", { month: "long", year: "numeric" }) ?? value;

  return (
    <nav className="calorieWeekPicker" aria-label={`Дни недели, ${weekLabel}`}>
      <button type="button" className="calorieWeekArrow" onClick={() => onChange(addDaysToDateKey(value, -7))} aria-label="Предыдущая неделя">‹</button>
      <div className="calorieWeekDays">
        {dates.map((date) => (
          <button
            type="button"
            key={date.value}
            className={`${date.value === value ? "isSelected " : ""}${date.value === localTodayKey() ? "isToday" : ""}`.trim()}
            aria-current={date.value === value ? "date" : undefined}
            onClick={() => onChange(date.value)}
          >
            <span>{date.label}</span>
            <strong>{Number(date.value.slice(-2))}</strong>
          </button>
        ))}
      </div>
      <button type="button" className="calorieWeekArrow" disabled={weekStart >= currentWeekStart} onClick={() => onChange(addDaysToDateKey(value, 7))} aria-label="Следующая неделя">›</button>
    </nav>
  );
}

function CaloriesSurface({ standalone, onClose, children }: { standalone: boolean; onClose: () => void; children: React.ReactNode }) {
  if (standalone) {
    return (
      <section className="caloriesPage" aria-labelledby="calories-page-title">
        <header className="caloriesPageHeader"><span className="caloriesPageIcon"><FlameIcon /></span><div><h1 id="calories-page-title">Калории</h1><p>Питание и КБЖУ внутри Identity Workspace</p></div></header>
        {children}
      </section>
    );
  }
  return <Modal title="Калории и КБЖУ" wide onClose={onClose}>{children}</Modal>;
}

function Modal({ onClose, title, wide = false, className = "", children }: { onClose: () => void; title: string; wide?: boolean; className?: string; children: React.ReactNode }) {
  const [viewportStyle, setViewportStyle] = useState<CSSProperties>();
  const isFoodPicker = className.split(/\s+/).includes("foodPickerModal");

  useEffect(() => {
    const viewport = window.visualViewport;
    if (!viewport) return;
    const syncViewport = () => {
      setViewportStyle({
        "--modal-viewport-height": `${viewport.height}px`,
        "--modal-viewport-offset-top": `${viewport.offsetTop}px`,
      } as CSSProperties);
    };
    syncViewport();
    viewport.addEventListener("resize", syncViewport);
    viewport.addEventListener("scroll", syncViewport);
    return () => {
      viewport.removeEventListener("resize", syncViewport);
      viewport.removeEventListener("scroll", syncViewport);
    };
  }, []);

  return (
    <div className={`overlay ${isFoodPicker ? "foodPickerOverlay" : ""}`.trim()} style={viewportStyle} onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className={`modal ${wide ? "modalWide" : ""} ${className}`.trim()}>
        <div className="modalHead"><span>{title}</span><button onClick={onClose} aria-label="Закрыть">×</button></div>
        {children}
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="field"><span className="fieldLabel">{label}</span>{children}</label>;
}

function EmptyState({ title, text, action, onAction }: { title: string; text: string; action?: string; onAction?: () => void }) {
  return <div className="emptyState"><span>◎</span><h2>{title}</h2><p>{text}</p>{action && onAction && <button className="primaryButton" onClick={onAction}>{action}</button>}</div>;
}

function ToastStack({ items, onDismiss }: { items: ToastMessage[]; onDismiss: (id: number) => void }) {
  const visibleItems = items.filter((item) => item.tone === "error");
  if (visibleItems.length === 0) return null;
  return (
    <div className="toastStack" aria-live="assertive">
      {visibleItems.map((item) => (
        <button className={`toast toast-${item.tone}`} key={item.id} onClick={() => onDismiss(item.id)}>
          <span>{item.tone === "error" ? "!" : item.tone === "success" ? "✓" : "·"}</span>
          <span><strong>{item.title}</strong>{item.detail && <small>{item.detail}</small>}</span>
        </button>
      ))}
    </div>
  );
}
