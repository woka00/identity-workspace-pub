import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  api,
  ApiError,
  AuthUser,
  CustomTracker,
  CustomTrackerEntry,
  CustomTrackerInput,
  FatSecretNutrition,
  FatSecretStatus,
  Goal,
  GoalInput,
  PortfolioResponse,
  Profile,
  Task,
  TaskCategory,
  TaskInput,
  TaskStatus,
  TickTickStatus,
  TrackerState,
  TrackerReminder,
  TrackerWaterEntry,
  TrackerWeightEntry,
} from "./api";

type Page = "card" | "tasks" | "projects" | "profile";
type ToastTone = "success" | "info" | "error";
type ProfileField = "name" | "surname" | "occupation" | "sex" | "dob" | "expiry";
type TrackerCardID = "calories" | "water" | "weight";
type TrackerSelectionID = TrackerCardID | `custom:${number}`;
type TrackerModal = "picker" | "reminders" | "statistics" | "calories" | "water" | "weight" | "custom-create" | "custom" | null;
type FatSecretCallbackStatus = "connected" | "denied" | "expired" | "error";
type TickTickCallbackStatus = "connected" | "denied" | "expired" | "error";

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

interface FatSecretNotice {
  tone: "success" | "error";
  text: string;
}

interface TickTickNotice {
  tone: "success" | "error";
  text: string;
}

const TRACKER_STORAGE_BASE = "identity-workspace.trackers.v1";
const TRACKER_SERVER_IMPORT_BASE = "identity-workspace.trackers.server-import.v1";
const TRACKER_LEGACY_CLAIM_KEY = "identity-workspace.trackers.legacy-claimed-by";
const trackerStorageKey = (userID: number) => `${TRACKER_STORAGE_BASE}.user-${userID}`;
const trackerServerImportKey = (userID: number) => `${TRACKER_SERVER_IMPORT_BASE}.user-${userID}`;
const MAX_TRACKER_CARDS = 6;
const WATER_GLASS_ML = 250;
const FORMAL_DAY_START_HOUR = 3;
const TRACKER_CARD_ORDER: TrackerCardID[] = ["calories", "water", "weight"];
const TRACKER_CARD_META: Record<TrackerCardID, { title: string; description: string }> = {
  calories: { title: "Калории", description: "Съедено за сегодня" },
  water: { title: "Вода", description: "Выпитые стаканы" },
  weight: { title: "Вес", description: "Последнее измерение" },
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

const TICKTICK_PROMO_STORAGE_BASE = "identity-workspace.ticktick-promo-dismissed.v1";

function tickTickPromoStorageKey(userID: number) {
  return `${TICKTICK_PROMO_STORAGE_BASE}.user-${userID}`;
}

function loadTickTickPromoDismissed(userID: number) {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem(tickTickPromoStorageKey(userID)) === "1";
}

function initialPage(): Page {
  if (typeof window === "undefined") return "card";
  const requested = new URL(window.location.href).searchParams.get("view");
  return requested === "tasks" || requested === "projects" || requested === "profile" || requested === "card"
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

function formatNutrition(value: number, maximumFractionDigits = 1) {
  return value.toLocaleString("ru-RU", { minimumFractionDigits: 0, maximumFractionDigits });
}

function fatSecretMealLabel(meal: string) {
  const labels: Record<string, string> = {
    Breakfast: "Завтрак",
    Lunch: "Обед",
    Dinner: "Ужин",
    Other: "Перекусы",
  };
  return labels[meal] ?? meal;
}

function consumeFatSecretCallbackNotice(): FatSecretNotice | null {
  if (typeof window === "undefined") return null;
  const url = new URL(window.location.href);
  const status = url.searchParams.get("fatsecret") as FatSecretCallbackStatus | null;
  if (!status) return null;

  url.searchParams.delete("fatsecret");
  window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);

  if (status === "connected") {
    return { tone: "success", text: "Существующий аккаунт FatSecret успешно подключён." };
  }
  if (status === "denied") {
    return { tone: "error", text: "Вы не подтвердили доступ к аккаунту FatSecret." };
  }
  if (status === "expired") {
    return { tone: "error", text: "Срок попытки подключения истёк. Запустите вход ещё раз." };
  }
  return { tone: "error", text: "Не удалось подключить аккаунт FatSecret. Повторите попытку." };
}

function consumeTickTickCallbackNotice(): TickTickNotice | null {
  if (typeof window === "undefined") return null;
  const url = new URL(window.location.href);
  const status = url.searchParams.get("ticktick") as TickTickCallbackStatus | null;
  if (!status) return null;

  url.searchParams.delete("ticktick");
  window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);

  if (status === "connected") return { tone: "success", text: "Аккаунт TickTick подключён. Задачи будут синхронизироваться в обе стороны автоматически." };
  if (status === "denied") return { tone: "error", text: "Вы не подтвердили доступ к аккаунту TickTick." };
  if (status === "expired") return { tone: "error", text: "Срок попытки подключения TickTick истёк. Запустите подключение ещё раз." };
  return { tone: "error", text: "Не удалось подключить TickTick. Повторите попытку." };
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

const emptyPortfolio: PortfolioResponse = { pinned: [], completed: [] };


export default function App() {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [checking, setChecking] = useState(true);
  const [serverError, setServerError] = useState("");

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
    window.addEventListener("identity-workspace:unauthorized", unauthorized);
    return () => window.removeEventListener("identity-workspace:unauthorized", unauthorized);
  }, [checkSession]);

  async function logout() {
    try {
      await unregisterDeviceNotifications();
      await api.logout();
    } finally {
      setUser(null);
    }
  }

  if (checking) return <div className="boot">ПРОВЕРКА ДОСТУПА<span className="blink">▌</span></div>;
  if (!user) {
    return <AuthScreen serverError={serverError} onAuthenticated={(nextUser) => { setServerError(""); setUser(nextUser); }} onRetry={checkSession} />;
  }
  return <AuthenticatedApp user={user} onLogout={logout} />;
}

function AuthScreen({ serverError, onAuthenticated, onRetry }: {
  serverError: string;
  onAuthenticated: (user: AuthUser) => void;
  onRetry: () => Promise<void>;
}) {
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const response = await api.login(login, password);
      onAuthenticated(response.user);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось выполнить вход");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="authPage">
      <section className="authDocument" aria-labelledby="auth-title">
        <header className="authDocumentHead">
          <span>identity workspace</span>
          <span>LIMITED ACCESS</span>
        </header>
        <div className="authDocumentBody">
          <div className="authSerial">FORM · A-01</div>
          <h1 id="auth-title">Вход</h1>
          <p>Приложение находится в закрытой разработке. Введите выданные логин и пароль.</p>

          <form className="authForm" onSubmit={submit}>
            <Field label="Логин">
              <input className="input authInput" value={login} onChange={(event) => setLogin(event.target.value)} autoComplete="username" autoCapitalize="none" spellCheck={false} autoFocus maxLength={32} required />
            </Field>
            <Field label="Пароль">
              <input className="input authInput" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" minLength={8} maxLength={128} required />
            </Field>
            {(error || serverError) && <div className="formError">{error || serverError}</div>}
            {serverError && <button type="button" className="textButton authRetry" onClick={() => void onRetry()}>Проверить сервер ещё раз</button>}
            <button className="primaryButton authSubmit" disabled={submitting}>{submitting ? "ПРОВЕРКА…" : "ВОЙТИ"}</button>
          </form>
        </div>
        <footer className="authDocumentFoot"><span>SESSION · HTTP-ONLY</span><span>PRIVATE PREVIEW</span></footer>
      </section>
    </main>
  );
}

function AuthenticatedApp({ user, onLogout }: { user: AuthUser; onLogout: () => Promise<void> }) {
  const [page, setPage] = useState<Page>(initialPage);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [currentDate, setCurrentDate] = useState("");
  const [tasks, setTasks] = useState<Task[]>([]);
  const [goals, setGoals] = useState<Goal[]>([]);
  const [portfolio, setPortfolio] = useState<PortfolioResponse>(emptyPortfolio);
  const [profileEditing, setProfileEditing] = useState<ProfileField | null>(null);
  const [signatureEditing, setSignatureEditing] = useState(false);
  const [taskEditing, setTaskEditing] = useState<Task | null>(null);
  const [goalEditing, setGoalEditing] = useState<Goal | "new" | null>(null);
  const [projectOrderBusy, setProjectOrderBusy] = useState(false);
  const [showTimeline, setShowTimeline] = useState(false);
  const [tickTickOpen, setTickTickOpen] = useState(false);
  const [tickTickWhyOpen, setTickTickWhyOpen] = useState(false);
  const [tickTickPromoDismissed, setTickTickPromoDismissed] = useState(() => loadTickTickPromoDismissed(user.id));
  const [tickTickStatus, setTickTickStatus] = useState<TickTickStatus | null>(null);
  const [tickTickBusy, setTickTickBusy] = useState(false);
  const [tickTickNotice] = useState<TickTickNotice | null>(consumeTickTickCallbackNotice);
  const [trackersEnabled, setTrackersEnabled] = useState(() => loadTrackerSettings(user.id).enabled);
  const tickTickSyncing = useRef(false);
  const [photoProcessing, setPhotoProcessing] = useState(false);
  const [photoProgress, setPhotoProgress] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  const toastID = useRef(0);
  const fileRef = useRef<HTMLInputElement>(null);

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

  useEffect(() => {
    if (tickTickNotice) addToast(tickTickNotice.tone === "success" ? "TickTick подключён" : "TickTick", tickTickNotice.text, tickTickNotice.tone);
  }, [addToast, tickTickNotice]);

  const load = useCallback(async () => {
    try {
      const [state, loadedTasks, loadedGoals, loadedPortfolio, loadedTickTick] = await Promise.all([
        api.state(),
        api.tasks(),
        api.goals(),
        api.portfolio(),
        api.tickTickStatus(),
      ]);
      setProfile(state.profile);
      setCurrentDate(state.currentDate);
      setTasks(loadedTasks);
      setGoals(loadedGoals);
      setPortfolio(loadedPortfolio);
      setTickTickStatus(loadedTickTick);
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "сервер недоступен");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const activeCount = tasks.filter((task) => task.status !== "done").length;
    document.title = `${activeCount} задач · identity workspace`;
  }, [tasks]);

  useEffect(() => {
    const modalOpen = profileEditing !== null || signatureEditing || taskEditing !== null || goalEditing !== null || showTimeline || tickTickOpen || tickTickWhyOpen;
    if (!modalOpen) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [profileEditing, signatureEditing, taskEditing, goalEditing, showTimeline, tickTickOpen, tickTickWhyOpen]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setProfileEditing(null);
        setSignatureEditing(false);
        setTaskEditing(null);
        setGoalEditing(null);
        setShowTimeline(false);
        setTickTickOpen(false);
        setTickTickWhyOpen(false);
        return;
      }
      const target = event.target as HTMLElement | null;
      if (
        event.metaKey || event.ctrlKey || event.altKey || target?.isContentEditable ||
        ["INPUT", "TEXTAREA", "SELECT"].includes(target?.tagName ?? "")
      ) return;
      if (event.key === "1") setPage("card");
      if (event.key === "2") setPage("tasks");
      if (event.key === "3") setPage("projects");
      if (event.key === "4") setPage("profile");
      if (event.key.toLowerCase() === "n" && page === "projects") setGoalEditing("new");
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [page]);

  const refreshTickTickStatus = useCallback(async () => {
    const status = await api.tickTickStatus();
    setTickTickStatus(status);
    return status;
  }, []);

  const syncTickTick = useCallback(async (showNotice = false) => {
    if (tickTickSyncing.current) return;
    tickTickSyncing.current = true;
    if (showNotice) setTickTickBusy(true);
    try {
      const result = await api.syncTickTick();
      const [loadedTasks, status] = await Promise.all([api.tasks(), api.tickTickStatus()]);
      setTasks(loadedTasks);
      setTickTickStatus(status);
      if (showNotice) {
        const details = [
          result.synced ? `отправлено: ${result.synced}` : "",
          result.imported ? `импортировано: ${result.imported}` : "",
          result.updated ? `обновлено: ${result.updated}` : "",
          result.completed ? `завершено: ${result.completed}` : "",
        ].filter(Boolean).join(" · ");
        if (result.failed > 0) addToast("TickTick синхронизирован частично", `${details ? `${details} · ` : ""}ошибок отправки: ${result.failed}`, "error");
        else addToast("TickTick синхронизирован", details || "Новых изменений нет.", "success");
      }
    } catch (cause) {
      if (showNotice) flashError(cause);
    } finally {
      tickTickSyncing.current = false;
      if (showNotice) setTickTickBusy(false);
    }
  }, [addToast, flashError]);

  useEffect(() => {
    if (!tickTickStatus?.connected) return;

    let stopped = false;
    let timer: number | undefined;
    const clearTimer = () => {
      if (timer !== undefined) window.clearTimeout(timer);
      timer = undefined;
    };
    const run = async () => {
      clearTimer();
      if (stopped || document.visibilityState !== "visible") return;
      await syncTickTick(false);
      if (!stopped && document.visibilityState === "visible") {
        timer = window.setTimeout(() => void run(), 30_000);
      }
    };
    const onVisibility = () => {
      clearTimer();
      if (!stopped && document.visibilityState === "visible") void run();
    };
    const onPageHide = () => clearTimer();

    if (document.visibilityState === "visible") void run();
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("pagehide", onPageHide);
    window.addEventListener("pageshow", onVisibility);
    return () => {
      stopped = true;
      clearTimer();
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("pagehide", onPageHide);
      window.removeEventListener("pageshow", onVisibility);
    };
  }, [syncTickTick, tickTickStatus?.connected]);

  async function connectTickTick() {
    const returnURL = new URL(window.location.href);
    returnURL.searchParams.delete("ticktick");
    returnURL.searchParams.set("view", "profile");
    const returnTo = `${returnURL.pathname}${returnURL.search}${returnURL.hash}`;
    try {
      const { authorizeUrl } = await api.connectTickTick(returnTo);
      window.location.assign(authorizeUrl);
    } catch (cause) {
      flashError(cause);
    }
  }

  function dismissTickTickPromo() {
    try {
      window.localStorage.setItem(tickTickPromoStorageKey(user.id), "1");
    } catch {
      // Скрываем предложение хотя бы до перезагрузки, если localStorage недоступен.
    }
    setTickTickPromoDismissed(true);
    setTickTickWhyOpen(false);
  }

  async function disconnectTickTick() {
    if (!window.confirm("Отключить TickTick? Уже отправленные задачи останутся в TickTick.")) return;
    setTickTickBusy(true);
    try {
      await api.disconnectTickTick();
      await refreshTickTickStatus();
      setTasks(await api.tasks());
      addToast("TickTick отключён", "Автоматическая синхронизация задач остановлена.", "info");
    } catch (cause) {
      flashError(cause);
    } finally {
      setTickTickBusy(false);
    }
  }

  async function refreshPortfolioAndGoals() {
    const [loadedGoals, loadedPortfolio] = await Promise.all([api.goals(), api.portfolio()]);
    setGoals(loadedGoals);
    setPortfolio(loadedPortfolio);
  }

  async function createTask(input: TaskInput) {
    try {
      const task = await api.createTask(input);
      setTasks((items) => [task, ...items]);
      // Не показываем popup после создания задачи; состояние TickTick видно в списке/статусе интеграции.
      if (tickTickStatus?.connected) void refreshTickTickStatus();
      return true;
    } catch (cause) {
      flashError(cause);
      return false;
    }
  }

  async function saveTask(input: TaskInput) {
    if (!taskEditing) return;
    try {
      const task = await api.updateTask(taskEditing.id, input);
      setTasks((items) => items.map((item) => item.id === task.id ? task : item));
      setTaskEditing(null);
      // Рутинное сохранение задачи не сопровождается всплывающим уведомлением.
      if (tickTickStatus?.connected) void refreshTickTickStatus();
    } catch (cause) {
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
      });
      setTasks((items) => items.map((item) => item.id === updated.id ? updated : item));
      if (tickTickStatus?.connected) void refreshTickTickStatus();
    } catch (cause) {
      flashError(cause);
    }
  }

  function setTrackerVisibility(enabled: boolean) {
    setTrackersEnabled(enabled);
    persistTrackerVisibility(user.id, enabled);
  }

  async function setTaskStatus(task: Task, status: TaskStatus) {
    try {
      const updated = status === "done"
        ? await api.completeTask(task.id)
        : await api.uncompleteTask(task.id);
      setTasks((items) => items.map((item) => item.id === updated.id ? updated : item));
      // Рутинное изменение статуса не сопровождается всплывающим уведомлением.
      if (tickTickStatus?.connected) void refreshTickTickStatus();
    } catch (cause) {
      flashError(cause);
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

  async function updateGoal(goal: Goal, patch: Partial<GoalInput>, message: string) {
    try {
      const input = { ...goalInput(goal), ...patch };
      if (input.pinned && !input.completed) input.pinned = false;
      const updated = await api.updateGoal(goal.id, input);
      await refreshPortfolioAndGoals();
      addToast(message, updated.title, "success");
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

  async function moveGoal(goal: Goal, direction: -1 | 1) {
    if (projectOrderBusy) return;
    const currentIndex = goals.findIndex((item) => item.id === goal.id);
    const targetIndex = currentIndex + direction;
    if (currentIndex < 0 || targetIndex < 0 || targetIndex >= goals.length) return;
    const previous = goals;
    const next = [...goals];
    [next[currentIndex], next[targetIndex]] = [next[targetIndex], next[currentIndex]];
    setGoals(next);
    setProjectOrderBusy(true);
    try {
      const ordered = await api.reorderGoals(next.map((item) => item.id));
      setGoals(ordered);
      setPortfolio(await api.portfolio());
    } catch (cause) {
      setGoals(previous);
      flashError(cause);
    } finally {
      setProjectOrderBusy(false);
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
    if (!window.confirm("Удалить все задачи и проекты? Профиль и фотография сохранятся.")) return;
    try {
      await api.reset();
      await load();
      addToast("Рабочие данные сброшены", "Профиль и фотография сохранены.", "info");
    } catch (cause) {
      flashError(cause);
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

  const activeTasks = tasks.filter((task) => task.status !== "done").length;
  const activeProjects = goals.filter((goal) => !goal.completed).length;

  return (
    <div className="root">
      <main className={`sheet page-${page}`}>
        <nav className="tabs" aria-label="Основные разделы">
          <Tab active={page === "card"} icon={<CardNavIcon />} label="Карта" meta="ID" onClick={() => setPage("card")} />
          <Tab active={page === "tasks"} icon={<TasksNavIcon />} label="Задачи" meta={String(activeTasks)} onClick={() => setPage("tasks")} />
          <Tab active={page === "projects"} icon={<ProjectsNavIcon />} label="Проекты" meta={String(activeProjects)} onClick={() => setPage("projects")} />
          <Tab active={page === "profile"} icon={<ProfileNavIcon />} label="Профиль" meta={profile.name.slice(0, 1).toUpperCase() || "ID"} onClick={() => setPage("profile")} />
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

            {trackersEnabled && <TrackerPreview currentDate={currentDate} userID={user.id} />}

            <section className="doc portfolioDigest">
              <header className="docTitle">
                <span>Портфолио</span>
                <span className="docTitleEn">{portfolio.completed.length} завершено</span>
              </header>
              {portfolio.completed.length === 0 ? (
                <EmptyState title="Портфолио пока пусто" text="Завершите первый крупный проект — он появится в хронологии карты." />
              ) : (
                <div className="digestRows">
                  {portfolio.completed.slice(0, 3).map((goal) => (
                    <div className="digestRow" key={goal.id}>
                      <time>{formatDate(goal.completedAt)}</time>
                      <strong>{goal.title}</strong>
                      <span>{goal.summary || goal.description || "Завершённый проект"}</span>
                    </div>
                  ))}
                </div>
              )}
              <button className="textButton timelineOpen" onClick={() => setShowTimeline(true)}>
                Открыть полный таймлайн →
              </button>
            </section>
          </>
        )}

        {page === "tasks" && (
          <TasksPage
            tasks={tasks}
            currentDate={currentDate}
            onCreate={createTask}
            onEdit={setTaskEditing}
            onStatus={setTaskStatus}
            onMoveToToday={moveTaskToToday}
            onDelete={deleteTask}
            showTickTickPromo={tickTickStatus !== null && !tickTickStatus.connected && !tickTickPromoDismissed}
            onTickTickWhy={() => setTickTickWhyOpen(true)}
            onDismissTickTickPromo={dismissTickTickPromo}
          />
        )}

        {page === "projects" && (
          <ProjectsPage
            goals={goals}
            tasks={tasks}
            onNew={() => setGoalEditing("new")}
            onEdit={setGoalEditing}
            onComplete={(goal) => updateGoal(goal, {
              completed: true,
              currentValue: Math.max(goal.currentValue, goal.targetValue),
            }, "Проект завершён")}
            onReopen={(goal) => updateGoal(goal, { completed: false, pinned: false }, "Проект возвращён в работу")}
            onMove={moveGoal}
            orderBusy={projectOrderBusy}
          />
        )}

        {page === "profile" && (
          <ProfilePage
            user={user}
            profile={profile}
            tickTickStatus={tickTickStatus}
            trackersEnabled={trackersEnabled}
            onOpenCard={() => setPage("card")}
            onTickTick={() => setTickTickOpen(true)}
            onTrackersEnabledChange={setTrackerVisibility}
          />
        )}

        {error && <div className="errorBar">{error}</div>}

        <footer className="foot">
          <span>identity workspace · {user.login.toUpperCase()}</span>
          <div className="footActions"><button className="reset" onClick={() => void onLogout()}>ВЫЙТИ</button><button className="reset resetDanger" onClick={resetAll}>СБРОСИТЬ ЗАДАЧИ И ПРОЕКТЫ</button></div>
        </footer>
      </main>

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

      {tickTickOpen && (
        <TickTickDialog
          status={tickTickStatus}
          busy={tickTickBusy}
          notice={tickTickNotice}
          onClose={() => setTickTickOpen(false)}
          onConnect={connectTickTick}
          onSync={() => void syncTickTick(true)}
          onDisconnect={() => void disconnectTickTick()}
        />
      )}

      {tickTickWhyOpen && (
        <TickTickWhyDialog
          onClose={() => setTickTickWhyOpen(false)}
          onDismiss={dismissTickTickPromo}
          onOpenProfile={() => {
            setTickTickWhyOpen(false);
            setPage("profile");
          }}
        />
      )}

      <ToastStack items={toasts} onDismiss={(id) => setToasts((items) => items.filter((item) => item.id !== id))} />
    </div>
  );
}

function TrackerPreview({ currentDate, userID }: { currentDate: string; userID: number }) {
  const trackerDate = currentDate || localTodayKey();
  const [legacySnapshot] = useState(() => loadLegacyTrackerSnapshot(userID));
  const [selected, setSelected] = useState<TrackerSelectionID[]>(legacySnapshot.settings.selected);
  const [trackerData, setTrackerData] = useState<TrackerState | null>(null);
  const [trackerLoading, setTrackerLoading] = useState(true);
  const [trackerError, setTrackerError] = useState<string | null>(null);
  const [fatSecretStatus, setFatSecretStatus] = useState<FatSecretStatus | null>(null);
  const [nutrition, setNutrition] = useState<FatSecretNutrition | null>(null);
  const [nutritionLoading, setNutritionLoading] = useState(true);
  const [nutritionError, setNutritionError] = useState<string | null>(null);
  const [fatSecretDisconnecting, setFatSecretDisconnecting] = useState(false);
  const [fatSecretNotice, setFatSecretNotice] = useState<FatSecretNotice | null>(consumeFatSecretCallbackNotice);
  const [calorieGoalSaving, setCalorieGoalSaving] = useState(false);
  const [calorieGoalError, setCalorieGoalError] = useState<string | null>(null);
  const [draftCalorieGoal, setDraftCalorieGoal] = useState(2000);
  const [waterSaving, setWaterSaving] = useState(false);
  const [weightSaving, setWeightSaving] = useState(false);
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
  const [trackerReminderDraft, setTrackerReminderDraft] = useState<Record<string, { enabled: boolean; time: string }>>({});
  const [trackerReminderLoading, setTrackerReminderLoading] = useState(false);
  const [trackerReminderSaving, setTrackerReminderSaving] = useState(false);
  const [trackerReminderError, setTrackerReminderError] = useState<string | null>(null);
  const [trackerNotificationConfig, setTrackerNotificationConfig] = useState<{ configured: boolean; publicKey: string } | null>(null);
  const [trackerNotificationState, setTrackerNotificationState] = useState<DeviceNotificationState>("unknown");
  const trackerReady = trackerData !== null;
  const waterEntry = trackerData?.waterHistory.find((entry) => entry.date === trackerDate);
  const waterGoal = trackerData?.waterGoal ?? legacySnapshot.settings.waterGoal;
  const calorieGoal = trackerData?.calorieGoal ?? 2000;
  const waterCount = trackerData ? (waterEntry?.glasses ?? 0) : (legacySnapshot.settings.waterByDate[trackerDate] ?? 0);
  const currentWeightKg = trackerData
    ? trackerData.currentWeightKg
    : (legacySnapshot.hasStoredWeight ? legacySnapshot.settings.weightKg : null);
  const selectedCards = selected.filter((id) => {
    if (TRACKER_CARD_ORDER.includes(id as TrackerCardID)) return true;
    const match = /^custom:(\d+)$/.exec(id);
    return Boolean(match && trackerData?.customTrackers.some((tracker) => tracker.id === Number(match[1])));
  });
  const activeCustomTracker = trackerData?.customTrackers.find((tracker) => tracker.id === activeCustomTrackerID) ?? null;
  const parsedWeight = Number(draftWeight.replace(",", "."));
  const weightIsValid = Number.isFinite(parsedWeight) && parsedWeight >= 20 && parsedWeight <= 500;
  const calorieGoalIsValid = Number.isInteger(draftCalorieGoal) && draftCalorieGoal >= 500 && draftCalorieGoal <= 10000;
  const lastFatSecretAutoRefresh = useRef(0);

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
    let active = true;

    async function loadFatSecret() {
      setNutritionLoading(true);
      setNutritionError(null);
      try {
        const status = await api.fatSecretStatus();
        if (!active) return;
        setFatSecretStatus(status);
        if (!status.connected) {
          setNutrition(null);
          return;
        }
        const dailyNutrition = await api.fatSecretNutrition(trackerDate);
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
  }, [trackerDate]);

  useEffect(() => {
    if (!fatSecretStatus?.connected) return;

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
        ...TRACKER_CARD_ORDER,
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
    setNutritionError(null);
    setCalorieGoalError(null);
    setDraftCalorieGoal(calorieGoal);
    setTrackerModal("calories");
  }

  async function connectExistingFatSecretAccount() {
    setFatSecretNotice(null);
    const returnURL = new URL(window.location.href);
    returnURL.searchParams.delete("fatsecret");
    const returnTo = `${returnURL.pathname}${returnURL.search}${returnURL.hash}`;
    try {
      const { authorizeUrl } = await api.connectFatSecret(returnTo);
      window.location.assign(authorizeUrl);
    } catch (cause) {
      setFatSecretNotice({ tone: "error", text: cause instanceof Error ? cause.message : "Не удалось начать подключение FatSecret." });
    }
  }

  async function refreshFatSecretNutrition(silent = false) {
    if (!fatSecretStatus?.connected || nutritionLoading) return;
    if (!silent) setNutritionLoading(true);
    setNutritionError(null);
    try {
      setNutrition(await api.fatSecretNutrition(trackerDate));
      lastFatSecretAutoRefresh.current = Date.now();
    } catch {
      setNutritionError("Не удалось обновить дневник FatSecret");
    } finally {
      if (!silent) setNutritionLoading(false);
    }
  }

  async function saveCalorieGoal() {
    if (!trackerData || calorieGoalSaving || !calorieGoalIsValid) return;
    setCalorieGoalSaving(true);
    setCalorieGoalError(null);
    try {
      const saved = await api.saveCalorieGoal(draftCalorieGoal);
      setTrackerData((current) => current ? { ...current, calorieGoal: saved.calorieGoal } : current);
      setDraftCalorieGoal(saved.calorieGoal);
    } catch {
      setCalorieGoalError("Не удалось сохранить дневную норму");
    } finally {
      setCalorieGoalSaving(false);
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

  async function createCustomTracker(event: React.FormEvent) {
    event.preventDefault();
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
      setTrackerError(cause instanceof Error ? cause.message : "Не удалось создать трекер");
    } finally { setCustomSaving(false); }
  }

  async function stepCustomTracker(tracker: CustomTracker, direction: -1 | 1) {
    if (customSaving) return;
    setCustomSaving(true);
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
      <section className="trackerPreview" aria-label="Трекеры">
        <header className="trackerToolbar">
          <div>
            <strong>Трекеры</strong>
            <span
              aria-live="polite"
              role={trackerError ? "alert" : "status"}
              style={trackerError ? { color: "var(--stamp)" } : undefined}
            >
              {trackerError ?? (trackerLoading ? "Синхронизация…" : `${selectedCards.length} из ${MAX_TRACKER_CARDS}`)}
            </span>
          </div>
          <div className="trackerToolbarActions">
            <button type="button" className="trackerSettingsButton trackerStatisticsButton" onClick={() => setTrackerModal("statistics")} disabled={!trackerReady} aria-label="Открыть статистику трекеров">
              <TrackerStatisticsIcon /><span>Статистика</span>
            </button>
            <button type="button" className="trackerSettingsButton" onClick={openPicker} aria-label="Настроить трекеры">
              <TrackerSlidersIcon /><span>Настроить</span>
            </button>
          </div>
        </header>

        {selectedCards.length === 0 && (
          <button type="button" className="trackerEmpty" onClick={openPicker}>
            <span aria-hidden="true">＋</span>
            <strong>Выберите трекер</strong>
            <small>Калории, вода или вес</small>
          </button>
        )}

        {selectedCards.map((card) => {
          if (card === "calories") {
            const caloriesValue = nutrition ? Math.round(nutrition.calories) : null;
            const statusPending = fatSecretStatus === null;
            const caloriesLabel = statusPending
              ? "проверка подключения…"
              : !fatSecretStatus.configured
                ? "нужны API-ключи"
                : !fatSecretStatus.connected
                  ? "подключить аккаунт"
                  : nutritionLoading && caloriesValue === null
                    ? "синхронизация…"
                    : nutritionError && caloriesValue === null
                      ? "нет данных"
                      : `из ${formatNutrition(calorieGoal, 0)} ккал`;
            const calorieProgress = caloriesValue === null
              ? 0
              : Math.min(100, Math.max(0, caloriesValue / calorieGoal * 100));
            const fatSecretDisconnected = !statusPending && !fatSecretStatus.connected;
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
                <button type="button" className="trackerMetricAction" onClick={openCaloriesTracker} aria-label={fatSecretDisconnected ? "Подключить FatSecret" : "Открыть калории и КБЖУ из FatSecret"} />
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
                <button type="button" className="trackerMetricAction" onClick={() => { setActiveCustomTrackerID(tracker.id); setTrackerModal("custom"); }} aria-label={`${tracker.name}: ${formatTrackerNumber(tracker.currentValue)} из ${formatTrackerNumber(tracker.targetValue)}. Открыть`} />
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
        <Modal title="Статистика трекеров" wide onClose={() => setTrackerModal(null)}>
          <TrackerStatistics
            currentDate={trackerDate}
            selected={selectedCards}
            state={trackerData}
            nutrition={nutrition}
            calorieGoal={calorieGoal}
          />
          <div className="modalActions"><button type="button" className="primaryButton" onClick={() => setTrackerModal(null)}>Готово</button></div>
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
            {TRACKER_CARD_ORDER.map((card) => {
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
            {(trackerData?.customTrackers ?? []).map((tracker) => {
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
            <button type="button" className="trackerChoice trackerChoiceCreate" onClick={() => setTrackerModal("custom-create")}>
              <span className="trackerChoiceIcon" aria-hidden="true">＋</span>
              <span className="trackerChoiceCopy"><strong>Создать свой трекер</strong><small>Название, цель, шаг и иконка</small></span>
              <span className="trackerChoiceCheck" aria-hidden="true">›</span>
            </button>
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
              {[...TRACKER_CARD_ORDER, ...(trackerData?.customTrackers ?? []).map((tracker) => `custom:${tracker.id}` as TrackerSelectionID)].map((card) => {
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


      {trackerModal === "custom-create" && (
        <Modal title="Новый трекер" onClose={() => setTrackerModal("picker")}>
          <form className="customTrackerForm" onSubmit={createCustomTracker}>
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
            <div className="modalActions"><button type="button" className="secondaryButton" onClick={() => setTrackerModal("picker")}>Назад</button><button className="primaryButton" disabled={customSaving || !customName.trim() || Number(customTarget.replace(",", ".")) <= 0 || Number(customStep.replace(",", ".")) <= 0 || Number(customStep.replace(",", ".")) > Number(customTarget.replace(",", "."))}>{customSaving ? "Создание…" : "Создать"}</button></div>
          </form>
        </Modal>
      )}

      {trackerModal === "custom" && activeCustomTracker && (
        <Modal title={activeCustomTracker.name} onClose={() => setTrackerModal(null)}>
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
          <div className="modalActions"><button type="button" className="dangerButton" disabled={customSaving} onClick={() => void deleteCustomTracker(activeCustomTracker)}>Удалить</button><button type="button" className="primaryButton" onClick={() => setTrackerModal(null)}>Готово</button></div>
        </Modal>
      )}

      {trackerModal === "calories" && (
        <Modal title="Калории и КБЖУ" onClose={() => setTrackerModal(null)}>
          {fatSecretNotice && (
            <div className={`fatSecretNotice fatSecretNotice-${fatSecretNotice.tone}`} role={fatSecretNotice.tone === "error" ? "alert" : "status"}>
              {fatSecretNotice.text}
            </div>
          )}
          {fatSecretStatus === null ? (
            <div className="nutritionSync" role="status">Проверяем подключение FatSecret…</div>
          ) : !fatSecretStatus.configured ? (
            <div className="fatSecretConnectState">
              <span className="fatSecretLogoMark" aria-hidden="true">FS</span>
              <strong>Добавьте ключи FatSecret Platform</strong>
              <p>В файле <code>.env</code> укажите <code>FATSECRET_CONSUMER_KEY</code> и <code>FATSECRET_CONSUMER_SECRET</code>, затем перезапустите Docker Compose.</p>
            </div>
          ) : !fatSecretStatus.connected ? (
            <div className="fatSecretConnectState">
              <span className="fatSecretLogoMark" aria-hidden="true">FS</span>
              <strong>Подключите существующий аккаунт</strong>
              <p>Вы войдёте в уже созданный аккаунт на сайте FatSecret и подтвердите доступ к пищевому дневнику. Логин и пароль не передаются identity workspace.</p>
              <button type="button" className="primaryButton fatSecretConnectButton" onClick={connectExistingFatSecretAccount}>Подключить существующий аккаунт</button>
              <small className="fatSecretExistingOnly">identity workspace не создаёт новые профили FatSecret.</small>
            </div>
          ) : (
            <>
              <div className="nutritionHero">
                <span>Съедено за день</span>
                <strong>{nutrition ? formatNutrition(nutrition.calories, 0) : "—"}<small> / {formatNutrition(calorieGoal, 0)} ккал</small></strong>
                <div className="nutritionGoalProgress" aria-label={`Выполнено ${nutrition ? Math.round(Math.min(100, nutrition.calories / calorieGoal * 100)) : 0}% дневной нормы`}>
                  <span style={{ width: `${nutrition ? Math.min(100, nutrition.calories / calorieGoal * 100) : 0}%` }} />
                </div>
                <div className="nutritionGoalStatus">
                  <time dateTime={trackerDate}>{formatDate(trackerDate)}</time>
                  <span>{nutrition ? (nutrition.calories <= calorieGoal
                    ? `Осталось ${formatNutrition(calorieGoal - nutrition.calories, 0)} ккал`
                    : `Сверх нормы ${formatNutrition(nutrition.calories - calorieGoal, 0)} ккал`) : ""}</span>
                </div>
              </div>

              <section className="calorieGoalEditor" aria-labelledby="calorie-goal-title">
                <div className="calorieGoalCopy">
                  <strong id="calorie-goal-title">Дневная норма</strong>
                  <span>Используется для прогресса на карточке калорий.</span>
                </div>
                <div className="calorieGoalControls">
                  <button type="button" onClick={() => setDraftCalorieGoal((value) => Math.max(500, value - 50))} disabled={draftCalorieGoal <= 500 || calorieGoalSaving} aria-label="Уменьшить норму калорий">−</button>
                  <label>
                    <span className="srOnly">Дневная норма калорий</span>
                    <input type="number" min="500" max="10000" step="50" inputMode="numeric" value={draftCalorieGoal} onChange={(event) => setDraftCalorieGoal(Number(event.target.value))} />
                    <small>ккал</small>
                  </label>
                  <button type="button" onClick={() => setDraftCalorieGoal((value) => Math.min(10000, value + 50))} disabled={draftCalorieGoal >= 10000 || calorieGoalSaving} aria-label="Увеличить норму калорий">＋</button>
                </div>
                <button type="button" className="secondaryButton calorieGoalSave" disabled={!trackerReady || !calorieGoalIsValid || calorieGoalSaving || draftCalorieGoal === calorieGoal} onClick={() => void saveCalorieGoal()}>
                  {calorieGoalSaving ? "Сохранение…" : draftCalorieGoal === calorieGoal ? "Норма сохранена" : "Сохранить норму"}
                </button>
                {!calorieGoalIsValid && <div className="formError" role="alert">Введите значение от 500 до 10 000 ккал.</div>}
                {calorieGoalError && <div className="formError" role="alert">{calorieGoalError}</div>}
              </section>

              <div className="macroGrid" aria-label="КБЖУ за день">
                <div><span>Белки</span><strong>{nutrition ? formatNutrition(nutrition.protein) : "—"}</strong><small>г</small></div>
                <div><span>Жиры</span><strong>{nutrition ? formatNutrition(nutrition.fat) : "—"}</strong><small>г</small></div>
                <div><span>Углеводы</span><strong>{nutrition ? formatNutrition(nutrition.carbohydrate) : "—"}</strong><small>г</small></div>
              </div>

              {nutrition && nutrition.meals.length > 0 ? (
                <div className="nutritionMeals">
                  <div className="nutritionSectionTitle"><strong>По приёмам пищи</strong><span>{nutrition.entryCount} записей</span></div>
                  {nutrition.meals.map((meal) => (
                    <div className="nutritionMeal" key={meal.meal}>
                      <div><strong>{fatSecretMealLabel(meal.meal)}</strong><span>{meal.entryCount} записей</span></div>
                      <strong>{formatNutrition(meal.calories, 0)} <small>ккал</small></strong>
                    </div>
                  ))}
                </div>
              ) : !nutritionLoading && !nutritionError ? (
                <p className="nutritionEmpty">В дневнике FatSecret за этот день пока нет записей.</p>
              ) : null}

              {nutritionLoading && <div className="nutritionSync" role="status">Обновляем дневник…</div>}
              {nutritionError && <div className="formError" role="alert">{nutritionError}</div>}
              <div className="modalActions fatSecretActions">
                <button type="button" className="secondaryButton" disabled={nutritionLoading || fatSecretDisconnecting} onClick={() => void disconnectFatSecret()}>Отключить</button>
                <button type="button" className="primaryButton" disabled={nutritionLoading || fatSecretDisconnecting} onClick={() => void refreshFatSecretNutrition()}>{nutritionLoading ? "Обновление…" : "Обновить"}</button>
              </div>
            </>
          )}
          <a className="fatSecretAttribution fatSecretAttributionBottom" href="https://platform.fatsecret.com" target="_blank" rel="noreferrer">Powered by fatsecret Platform API</a>
        </Modal>
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

function TrackerStatistics({
  currentDate,
  selected,
  state,
  nutrition,
  calorieGoal,
}: {
  currentDate: string;
  selected: TrackerSelectionID[];
  state: TrackerState;
  nutrition: FatSecretNutrition | null;
  calorieGoal: number;
}) {
  const [period, setPeriod] = useState<7 | 30>(7);
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
  const visibleTrackers = selected.length > 0
    ? selected
    : (["calories", "water", "weight"] as TrackerSelectionID[]);

  return (
    <div className="trackerStatistics">
      <header className="trackerStatisticsHeader">
        <div>
          <strong>Динамика по дням</strong>
          <span>{formatStatisticsRange(firstDate, currentDate)}</span>
        </div>
        <div className="trackerStatisticsPeriod" role="group" aria-label="Период статистики">
          <button type="button" className={period === 7 ? "active" : ""} aria-pressed={period === 7} onClick={() => setPeriod(7)}>7 дней</button>
          <button type="button" className={period === 30 ? "active" : ""} aria-pressed={period === 30} onClick={() => setPeriod(30)}>30 дней</button>
        </div>
      </header>

      <div className="trackerStatisticsGrid">
        {visibleTrackers.includes("calories") && (
          <article className="trackerStatisticsCard trackerStatisticsCalories">
            <header><span className="trackerStatisticsIcon"><FlameIcon /></span><div><strong>Калории</strong><small>{formatStatisticsDay(currentDate)}</small></div></header>
            <div className="trackerStatisticsGoal">
              <GoalDonut progress={calorieValue === null ? 0 : calorieValue / calorieGoal * 100} />
              <div><strong>{calorieValue === null ? "—" : formatNutrition(calorieValue, 0)}</strong><span>из {formatNutrition(calorieGoal, 0)} ккал</span></div>
            </div>
            <p>{calorieValue === null ? "Нет данных FatSecret за выбранный день." : calorieValue <= calorieGoal ? `Осталось ${formatNutrition(calorieGoal - calorieValue, 0)} ккал` : `Сверх нормы ${formatNutrition(calorieValue - calorieGoal, 0)} ккал`}</p>
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
            <DailyLineChart dates={dates} values={weightValues} period={period} suffix="кг" />
            <p>{weightDelta === null ? "Добавляйте вес по дням — здесь появится динамика." : weightDelta === 0 ? "Вес за период не изменился." : `Изменение за период: ${weightDelta > 0 ? "+" : ""}${formatTrackerWeight(weightDelta)} кг.`}</p>
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
          const values = cumulativeTrackerSeries(dates, entries, currentDate, tracker.currentValue);
          return (
            <article className="trackerStatisticsCard trackerStatisticsWide" key={selection}>
              <header><span className="trackerStatisticsIcon customTrackerGlyph"><CustomTrackerIcon icon={tracker.icon} /></span><div><strong>{tracker.name}</strong><small>Шаг {formatTrackerNumber(tracker.stepValue)}</small></div><b>{Math.round(Math.min(100, tracker.currentValue / tracker.targetValue * 100))}%</b></header>
              <DailyLineChart dates={dates} values={values} period={period} suffix="" target={tracker.targetValue} />
              <p>{formatTrackerNumber(tracker.currentValue)} из {formatTrackerNumber(tracker.targetValue)} · история пополняется при каждом изменении.</p>
            </article>
          );
        })}
      </div>
    </div>
  );
}

function DailyBarChart({ dates, values, goals, period, suffix }: {
  dates: string[];
  values: number[];
  goals: number[];
  period: 7 | 30;
  suffix: string;
}) {
  const maximum = Math.max(1, ...values, ...goals);
  return (
    <div className={`dailyBarChart dailyBarChart${period}`} role="img" aria-label="Столбчатый график воды по дням">
      {dates.map((date, index) => {
        const value = values[index];
        const goal = goals[index];
        const showLabel = period === 7 || index === 0 || index === dates.length - 1 || index % 5 === 4;
        return (
          <div className="dailyBarColumn" key={date} title={`${formatStatisticsDay(date)}: ${formatTrackerNumber(value)} ${suffix}, цель ${formatTrackerNumber(goal)}`}>
            <div className="dailyBarPlot">
              <i style={{ bottom: `${Math.min(100, goal / maximum * 100)}%` }} />
              <span className={value >= goal ? "goalReached" : ""} style={{ height: `${Math.max(value > 0 ? 5 : 0, value / maximum * 100)}%` }} />
            </div>
            <small>{showLabel ? formatStatisticsAxisDay(date, period) : ""}</small>
          </div>
        );
      })}
    </div>
  );
}

function DailyLineChart({ dates, values, period, suffix, target }: {
  dates: string[];
  values: Array<number | null>;
  period: 7 | 30;
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
        <path className="dailyLineGrid" d="M12 10H308M12 48H308M12 86H308" />
        {targetY !== null && <path className="dailyLineTarget" d={`M12 ${targetY}H308`} />}
        {points.length > 1 && <polyline points={polyline} />}
        {points.map((point) => <circle key={`${point.index}-${point.value}`} cx={x(point.index)} cy={y(point.value)} r={points.length === 1 ? 4 : 2.8} />)}
      </svg>
      <div className="dailyLineLabels"><span>{formatStatisticsAxisDay(dates[0], period)}</span><strong>{formatTrackerNumber(points[points.length - 1].value)}{suffix ? ` ${suffix}` : ""}</strong><span>{formatStatisticsAxisDay(dates[dates.length - 1], period)}</span></div>
    </div>
  );
}

function GoalDonut({ progress }: { progress: number }) {
  const safeProgress = Math.min(100, Math.max(0, progress));
  return (
    <div className="trackerStatisticsDonut" aria-label={`Выполнено ${Math.round(safeProgress)} процентов`}>
      <svg viewBox="0 0 54 54" aria-hidden="true">
        <circle cx="27" cy="27" r="21" pathLength="100" />
        <circle className="value" cx="27" cy="27" r="21" pathLength="100" strokeDasharray="100" strokeDashoffset={100 - safeProgress} />
      </svg>
      <strong>{Math.round(safeProgress)}%</strong>
    </div>
  );
}

function cumulativeTrackerSeries(dates: string[], entries: CustomTrackerEntry[], currentDate: string, currentValue: number) {
  let lastValue: number | null = null;
  let entryIndex = 0;
  return dates.map((date) => {
    while (entryIndex < entries.length && entries[entryIndex].date <= date) {
      lastValue = entries[entryIndex].value;
      entryIndex += 1;
    }
    if (date === currentDate) return currentValue;
    return lastValue;
  });
}

function formatStatisticsDay(value: string) {
  const date = parseDateKey(value);
  if (!date) return value;
  const formatted = date.toLocaleDateString("ru-RU", { weekday: "short", day: "numeric", month: "short" });
  return formatted.replace(".", "");
}

function formatStatisticsAxisDay(value: string, period: 7 | 30) {
  const date = parseDateKey(value);
  if (!date) return value.slice(8);
  return period === 7
    ? date.toLocaleDateString("ru-RU", { weekday: "short" }).replace(".", "")
    : date.toLocaleDateString("ru-RU", { day: "numeric", month: "short" }).replace(".", "");
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
  return <DumbbellIcon />;
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
  return <img className="trackerBuiltinIcon" src="/tracker-builtins/water-v2.png" alt="" aria-hidden="true" draggable={false} />;
}

function FlameIcon() {
  return <img className="trackerBuiltinIcon" src="/tracker-builtins/calories-v2.png" alt="" aria-hidden="true" draggable={false} />;
}

function DumbbellIcon() {
  return <img className="trackerBuiltinIcon" src="/tracker-builtins/weight-v2.png" alt="" aria-hidden="true" draggable={false} />;
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
  meta: string;
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
      <span className="tabMeta">{meta}</span>
    </button>
  );
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
      <rect x="4" y="4" width="6" height="6" rx="2" />
      <rect x="14" y="4" width="6" height="6" rx="2" />
      <rect x="4" y="14" width="6" height="6" rx="2" />
      <rect x="14" y="14" width="6" height="6" rx="2" />
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

function goalInput(goal: Goal): GoalInput {
  return {
    title: goal.title,
    description: goal.description,
    summary: goal.summary,
    currentValue: goal.currentValue,
    targetValue: goal.targetValue,
    unit: goal.unit,
    deadline: goal.deadline,
    relatedTaskIds: goal.relatedTaskIds,
    completed: goal.completed,
    pinned: goal.pinned,
  };
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

function parseDateKey(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return null;
  const [, year, month, day] = match;
  const parsed = new Date(Number(year), Number(month) - 1, Number(day), 12);
  if (
    parsed.getFullYear() !== Number(year) ||
    parsed.getMonth() !== Number(month) - 1 ||
    parsed.getDate() !== Number(day)
  ) return null;
  return parsed;
}

function dateKeyFromDate(value: Date) {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function localTodayKey() {
  return dateKeyFromDate(new Date());
}

function formalTodayKey(calendarDate = localTodayKey()) {
  return new Date().getHours() < FORMAL_DAY_START_HOUR ? addDaysToDateKey(calendarDate, -1) : calendarDate;
}

function dateKeyFromTimestamp(value: string) {
  if (!value) return "";
  // PostgreSQL historically returned offsets like +00, while iOS Safari expects
  // RFC3339 (+00:00 or Z). Normalize old values for iOS Safari, then shift by
  // the formal 03:00 boundary: 00:00–02:59 belong to the previous task day.
  const normalized = value
    .replace(/([+-]\d{2})$/, "$1:00")
    .replace(/([+-]\d{2})(\d{2})$/, "$1:$2");
  const parsed = new Date(normalized);
  if (Number.isNaN(parsed.getTime())) return "";
  parsed.setHours(parsed.getHours() - FORMAL_DAY_START_HOUR);
  return dateKeyFromDate(parsed);
}

function addDaysToDateKey(value: string, amount: number) {
  const parsed = parseDateKey(value);
  if (!parsed) return value;
  parsed.setDate(parsed.getDate() + amount);
  return dateKeyFromDate(parsed);
}

function startOfWeekDateKey(value: string) {
  const parsed = parseDateKey(value) ?? new Date();
  parsed.setHours(12, 0, 0, 0);
  const daysSinceMonday = (parsed.getDay() + 6) % 7;
  parsed.setDate(parsed.getDate() - daysSinceMonday);
  return dateKeyFromDate(parsed);
}

function startOfMonthDateKey(value: string) {
  const parsed = parseDateKey(value) ?? new Date();
  parsed.setDate(1);
  parsed.setHours(12, 0, 0, 0);
  return dateKeyFromDate(parsed);
}

function addMonthsToDateKey(value: string, amount: number) {
  const parsed = parseDateKey(value) ?? new Date();
  parsed.setDate(1);
  parsed.setMonth(parsed.getMonth() + amount);
  return dateKeyFromDate(parsed);
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
        <button className="cardData cardExpiry" onClick={() => onEdit("expiry")} aria-label="Изменить срок действия">{field(profile.expiry, "—")}</button>
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

function urlBase64ToUint8Array(value: string) {
  const padding = "=".repeat((4 - value.length % 4) % 4);
  const base64 = (value + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = window.atob(base64);
  return Uint8Array.from(raw, (character) => character.charCodeAt(0));
}

type DeviceNotificationState = "unknown" | "enabled" | "denied" | "unsupported" | "error";

async function unregisterDeviceNotifications() {
  if (!("serviceWorker" in navigator) || !("PushManager" in window)) return;
  try {
    const registration = await navigator.serviceWorker.ready;
    const subscription = await registration.pushManager.getSubscription();
    if (!subscription) return;
    try { await api.deletePushSubscription(subscription.endpoint); } catch { /* logout must continue */ }
    await subscription.unsubscribe();
  } catch {
    // The server session will still be closed even if the browser cannot remove its local subscription.
  }
}

async function registerDeviceNotifications(
  requestPermission = true,
  config?: { configured: boolean; publicKey: string },
): Promise<DeviceNotificationState> {
  try {
    const resolvedConfig = config ?? await api.notificationConfig();
    if (!resolvedConfig.configured || !resolvedConfig.publicKey || !("Notification" in window) || !("serviceWorker" in navigator) || !("PushManager" in window)) return "unsupported";
    const permission = requestPermission ? await Notification.requestPermission() : Notification.permission;
    if (permission !== "granted") return permission === "denied" ? "denied" : "unknown";
    const registration = await navigator.serviceWorker.ready;
    let subscription = await registration.pushManager.getSubscription();
    if (!subscription) {
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(resolvedConfig.publicKey),
      });
    }
    const json = subscription.toJSON();
    if (!json.endpoint || !json.keys?.p256dh || !json.keys.auth) return "error";
    await api.savePushSubscription({ endpoint: json.endpoint, p256dh: json.keys.p256dh, auth: json.keys.auth });
    return "enabled";
  } catch {
    return "error";
  }
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
  currentDate,
  onCreate,
  onEdit,
  onStatus,
  onMoveToToday,
  onDelete,
  showTickTickPromo,
  onTickTickWhy,
  onDismissTickTickPromo,
}: {
  tasks: Task[];
  currentDate: string;
  onCreate: (input: TaskInput) => Promise<boolean>;
  onEdit: (task: Task) => void;
  onStatus: (task: Task, status: TaskStatus) => Promise<void>;
  onMoveToToday: (task: Task, date: string) => Promise<void>;
  onDelete: (task: Task) => Promise<void>;
  showTickTickPromo: boolean;
  onTickTickWhy: () => void;
  onDismissTickTickPromo: () => void;
}) {
  const [todayKey, setTodayKey] = useState(() => formalTodayKey(currentDate || localTodayKey()));
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [category, setCategory] = useState("");
  const [dueDate, setDueDate] = useState(todayKey);
  const [dueTime, setDueTime] = useState("");
  const [reminderLocal, setReminderLocal] = useState("");
  const [priority, setPriority] = useState(0);
  const [selectedDate, setSelectedDate] = useState<string | null>(todayKey);
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
  const filterMenuRef = useRef<HTMLDivElement>(null);

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
  const availableCategories = useMemo(() => categories, [categories]);
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
      const leftKey = `${left.dueDate || "9999-12-31"}T${left.dueTime || "23:59"}`;
      const rightKey = `${right.dueDate || "9999-12-31"}T${right.dueTime || "23:59"}`;
      return leftKey.localeCompare(rightKey) || left.title.localeCompare(right.title, "ru");
    });
  }, [tasks, selectedCategory, selectedDate, todayKey]);

  const taskSections = useMemo(() => {
    if (selectedDate) {
      if (visibleTasks.length === 0) return [];
      if (selectedDate === todayKey) {
        const overdueTasks = visibleTasks.filter((task) => task.dueDate < todayKey);
        const currentTasks = visibleTasks.filter((task) => task.dueDate === todayKey);
        return [
          ...(overdueTasks.length > 0 ? [{ key: "overdue", title: "Просроченные", tasks: overdueTasks }] : []),
          ...(currentTasks.length > 0 ? [{ key: "todo", title: "Невыполненные", tasks: currentTasks }] : []),
        ];
      }
      return [{
        key: "todo",
        title: formatSelectedDate(selectedDate),
        tasks: visibleTasks,
      }];
    }
    const overdueTasks = visibleTasks.filter((task) => Boolean(task.dueDate) && task.dueDate < todayKey);
    const currentTasks = visibleTasks.filter((task) => !task.dueDate || task.dueDate >= todayKey);
    const grouped = new Map<string, Task[]>();
    currentTasks.forEach((task) => {
      const key = task.dueDate || "undated";
      grouped.set(key, [...(grouped.get(key) ?? []), task]);
    });
    return [
      ...(overdueTasks.length > 0 ? [{ key: "overdue", title: "Просроченные", tasks: overdueTasks }] : []),
      ...[...grouped.entries()].map(([date, items]) => ({
        key: date,
        title: date === "undated" ? "Без даты" : date === todayKey ? "Сегодня" : formatSelectedDate(date),
        tasks: items,
      })),
    ];
  }, [selectedDate, todayKey, visibleTasks]);

  const completedTasksForDay = useMemo(() => {
    if (!selectedDate) return [];
    return tasks
      .filter((task) => task.status === "done" && categoryMatches(task, selectedCategory) && dateKeyFromTimestamp(task.completedAt) === selectedDate)
      .sort((left, right) => right.completedAt.localeCompare(left.completedAt));
  }, [tasks, selectedCategory, selectedDate]);

  const activeCount = tasks.filter((task) => task.status === "todo").length;

  useEffect(() => {
    setSelectedDate(todayKey);
    setWeekStart(startOfWeekDateKey(todayKey));
  }, [todayKey]);

  useEffect(() => {
    setCompletedOpen(true);
  }, [selectedDate]);

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

  function openCreateForm() {
    if (createSubmitting) return;
    setCategory("");
    setPriority(0);
    setDueDate(selectedDate ?? todayKey);
    setCreateOpen(true);
  }

  const filterLabel = selectedCategory ?? "Категория";
  const emptyText = selectedDate
    ? `На ${formatSelectedDate(selectedDate).toLocaleLowerCase("ru-RU")} задач нет.`
    : selectedCategory
      ? `В категории «${selectedCategory}» нет активных задач.`
      : "Активных задач нет.";

  return (
    <section className="doc taskDocument">
      <header className="taskHeader">
        <div>
          <span className="taskKicker">Личный список</span>
          <h1>Задачи</h1>
          <time dateTime={todayKey}>{formatRegisterDate(todayKey)}</time>
        </div>
        <div className="taskHeaderMeta">
          <div className="taskOpenCount" aria-label={`${activeCount} активных задач`}>
            <strong>{activeCount}</strong><span>{activeTaskLabel(activeCount)}</span>
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
                  <form className="taskCategoryCreate" onSubmit={addCategory}><input value={newCategory} maxLength={40} placeholder="Новая категория" onChange={(event) => setNewCategory(event.target.value)} /><button disabled={!newCategory.trim() || categoryBusy}>＋</button></form>
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

      {showTickTickPromo && <div className="taskListLead"><aside className="tickTickPromo"><div><strong>Подключи TickTick</strong><button type="button" className="tickTickPromoWhy" onClick={onTickTickWhy}>Зачем мне это?</button></div><button type="button" className="tickTickPromoDismiss" onClick={onDismissTickTickPromo}>Больше не показывать</button></aside></div>}

      {taskSections.length === 0 && completedTasksForDay.length === 0 ? <EmptyState title={tasks.length ? "Здесь пока пусто" : "Список пуст"} text={tasks.length ? emptyText : "Создайте первую задачу."} /> : (
        <div className="taskList taskListCompact">
          {taskSections.map((section) => <section className="taskSection" key={section.key}><header className="taskSectionHeader"><span>{section.title}</span><small>{section.tasks.length}</small></header>{section.tasks.map((task) => <TaskRow key={task.id} task={task} currentDate={todayKey} onEdit={onEdit} onStatus={onStatus} onMoveToToday={onMoveToToday} onDelete={onDelete} />)}</section>)}
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

      <button type="button" className="createFab" disabled={createSubmitting} aria-busy={createSubmitting} onClick={openCreateForm}><span aria-hidden="true">＋</span> {createSubmitting ? "Создание…" : "Задача"}</button>
      {createOpen && <Modal title="Новая задача" onClose={() => setCreateOpen(false)}>
        <form className="mobileCreateForm" onSubmit={submit}>
          <label className="taskQuickTitle"><span>Что нужно сделать?</span><textarea value={title} maxLength={160} autoFocus rows={2} placeholder="Название задачи" onChange={(event) => setTitle(event.target.value)} /></label>
          <label className="field"><span className="fieldLabel">Описание</span><textarea className="input taskDescriptionInput" value={description} maxLength={2000} rows={3} placeholder="Необязательные подробности" onChange={(event) => setDescription(event.target.value)} /></label>
          <div className="taskFormGrid">
            <div className="field"><span className="fieldLabel">Категория</span><TaskCategoryPicker value={category} categories={availableCategories} onChange={setCategory} onCreate={createCategoryForTask} onDelete={deleteCategory} /></div>
            <div className="field"><span className="fieldLabel">Дата</span><TaskDateControl value={dueDate} currentDate={todayKey} allowUndated={Boolean(category.trim())} onChange={(value) => { setDueDate(value); if (!value) setDueTime(""); }} /></div>
            <div className="field"><span className="fieldLabel">Время задачи</span><TaskTimeControl value={dueTime} disabled={!dueDate} onChange={setDueTime} /></div>
            <label className="field"><span className="fieldLabel">Напомнить</span><input className="input" type="datetime-local" value={reminderLocal} onChange={(event) => setReminderLocal(event.target.value)} /></label>
          </div>
          <div className="field"><span className="fieldLabel">Приоритет</span><TaskPriorityPicker value={priority} onChange={setPriority} /></div>
          {reminderLocal && notificationState !== "enabled" && <div className="notificationSetup"><div><strong>Уведомления на устройство</strong><span>{notificationState === "denied" ? "Разрешение заблокировано в настройках браузера." : notificationState === "unsupported" ? "Этот браузер или режим не поддерживает Web Push." : "Разрешите уведомления, чтобы напоминание пришло даже при закрытом приложении."}</span></div>{!(["denied", "unsupported"] as string[]).includes(notificationState) && <button type="button" className="secondaryButton" onClick={() => void syncPushSubscription()}>Включить</button>}</div>}
          <div className="modalActions"><button type="button" className="secondaryButton" onClick={() => setCreateOpen(false)}>Отмена</button><button className="primaryButton" disabled={!title.trim() || (!dueDate && !category.trim())}>Создать задачу</button></div>
        </form>
      </Modal>}
    </section>
  );
}

function TaskRow({ task, currentDate, onEdit, onStatus, onMoveToToday, onDelete }: {
  task: Task;
  currentDate: string;
  onEdit: (task: Task) => void;
  onStatus: (task: Task, status: TaskStatus) => Promise<void>;
  onMoveToToday: (task: Task, date: string) => Promise<void>;
  onDelete: (task: Task) => Promise<void>;
}) {
  const [actionsOpen, setActionsOpen] = useState(false);
  const [completing, setCompleting] = useState(false);
  const completionTimer = useRef<number | null>(null);
  const overdue = task.status === "todo" && Boolean(task.dueDate) && task.dueDate < currentDate;
  const completedDateKey = task.status === "done" ? dateKeyFromTimestamp(task.completedAt) : "";
  const visibleCategory = task.category.trim().toLocaleLowerCase("ru-RU") === "ticktick" ? "" : task.category.trim();

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

  return <article className={`taskRow task-${task.status} taskPriority-${task.priority} ${actionsOpen ? "taskActionsOpen" : ""} ${completing ? "taskCompleting" : ""}`}>
    <button className="taskCheck" onClick={toggleStatusWithUndo} aria-label={`${completing ? "Отменить выполнение задачи" : task.status === "done" ? "Вернуть задачу" : "Выполнить задачу"}; приоритет ${task.priority}`} title={completing ? "Нажмите ещё раз, чтобы отменить" : undefined}>{task.status === "done" || completing ? "✓" : ""}</button>
    <button className="taskBody" onClick={() => onEdit(task)} disabled={completing} title={task.description || task.title}><span className="taskName">{task.title}</span></button>
    <div className="taskRowMeta">
      {completing ? <span className="taskUndoHint">Ещё раз — отменить</span> : <>
        {visibleCategory && <span className="taskCategory">{visibleCategory}</span>}
        {task.reminderAt && !task.reminderSentAt && <span className="taskReminderMark" title={`Напоминание: ${new Date(task.reminderAt).toLocaleString("ru-RU")}`}>◷</span>}
        {task.status === "done" && completedDateKey ? (
          <time className="taskDueDate taskCompletedDate" dateTime={task.completedAt}>{completedDateKey === currentDate ? "Сегодня" : formatTaskDueDate(completedDateKey, currentDate)}</time>
        ) : task.dueDate ? (
          <time className={`taskDueDate ${overdue ? "taskDueDateOverdue" : ""}`} dateTime={`${task.dueDate}${task.dueTime ? `T${task.dueTime}` : ""}`}>{formatTaskDueDate(task.dueDate, currentDate)}{task.dueTime ? ` · ${task.dueTime}` : ""}</time>
        ) : null}
      </>}
    </div>
    <button className="taskMore" disabled={completing} onClick={() => setActionsOpen((open) => !open)} aria-label="Действия с задачей" aria-expanded={actionsOpen}>⋯</button>
    {actionsOpen && <div className="taskActions">{task.status === "done" && <button onClick={() => void onStatus(task, "todo")}>Вернуть</button>}{overdue && <button onClick={() => { setActionsOpen(false); void onMoveToToday(task, currentDate); }}>Перенести на сегодня</button>}<button onClick={() => { setActionsOpen(false); onEdit(task); }}>Изменить</button><button className="taskDelete" onClick={() => { setActionsOpen(false); void onDelete(task); }}>Удалить</button></div>}
  </article>;
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

function ProjectsPage({
  goals,
  tasks,
  onNew,
  onEdit,
  onComplete,
  onReopen,
  onMove,
  orderBusy,
}: {
  goals: Goal[];
  tasks: Task[];
  onNew: () => void;
  onEdit: (goal: Goal) => void;
  onComplete: (goal: Goal) => Promise<void>;
  onReopen: (goal: Goal) => Promise<void>;
  onMove: (goal: Goal, direction: -1 | 1) => Promise<void>;
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
                      <button type="button" disabled={orderBusy || goals[0]?.id === goal.id} onClick={() => void onMove(goal, -1)} aria-label="Переместить проект выше">↑</button>
                      <button type="button" disabled={orderBusy || goals[goals.length - 1]?.id === goal.id} onClick={() => void onMove(goal, 1)} aria-label="Переместить проект ниже">↓</button>
                    </div>
                    <strong className="projectPercent">{goal.completionPct}%</strong>
                  </div>
                </div>
                <div className="projectProgress"><div style={{ width: `${goal.completionPct}%` }} /></div>
                <div className="projectNumbers">
                  {formatValue(goal.currentValue)} / {formatValue(goal.targetValue)} {goal.unit}
                </div>
                {related.length > 0 && (
                  <div className="relatedTasks">Связано: {related.map((task) => task.title).join(" · ")}</div>
                )}
                {goal.completed && <div className="projectSummary">{goal.summary || "Завершённый проект"}</div>}
                <div className="projectActions">
                  <button className="textButton" onClick={() => onEdit(goal)}>Изменить</button>
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

function TickTickDialog({
  status,
  busy,
  notice,
  onClose,
  onConnect,
  onSync,
  onDisconnect,
}: {
  status: TickTickStatus | null;
  busy: boolean;
  notice: TickTickNotice | null;
  onClose: () => void;
  onConnect: () => void;
  onSync: () => void;
  onDisconnect: () => void;
}) {
  return (
    <Modal onClose={onClose} title="Интеграция TickTick">
      <div className="tickTickDialog">
        {notice && <div className={`integrationNotice integrationNotice-${notice.tone}`}>{notice.text}</div>}
        {!status ? (
          <p>Проверяем состояние подключения…</p>
        ) : !status.configured ? (
          <>
            <p>Интеграция не настроена на сервере.</p>
            <div className="integrationCode">TICKTICK_CLIENT_ID<br />TICKTICK_CLIENT_SECRET</div>
            <p className="integrationHint">Добавьте значения в <code>.env</code> и перезапустите контейнер приложения. Базу данных удалять не нужно.</p>
          </>
        ) : !status.connected ? (
          <>
            <p>Подключите существующий аккаунт TickTick. identity workspace создаст отдельный проект для исходящих задач и будет импортировать активные задачи из всех списков TickTick.</p>
            <button type="button" className="primaryButton" onClick={onConnect}>Подключить TickTick</button>
          </>
        ) : (
          <>
            <div className="integrationStatusRow"><span>Статус</span><strong>Подключено</strong></div>
            <div className="integrationStatusRow"><span>Проект</span><strong>{status.projectName || "identity workspace"}</strong></div>
            <div className="integrationStatusRow"><span>Ожидают отправки</span><strong>{status.pendingTasks}</strong></div>
            <div className="integrationStatusRow"><span>С ошибкой</span><strong className={status.failedTasks ? "integrationErrorValue" : ""}>{status.failedTasks}</strong></div>
            <p className="integrationHint">Пока identity workspace открыт и вкладка видима, задачи обновляются из TickTick каждые 30 секунд. После закрытия или сворачивания вкладки опрос прекращается. Локальные изменения и отметка «выполнено» также отправляются в TickTick.</p>
            <div className="modalActions">
              <button type="button" className="secondaryButton" disabled={busy} onClick={onDisconnect}>Отключить</button>
              <button type="button" className="primaryButton" disabled={busy} onClick={onSync}>{busy ? "СИНХРОНИЗАЦИЯ…" : "Синхронизировать"}</button>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}

function TickTickWhyDialog({
  onClose,
  onDismiss,
  onOpenProfile,
}: {
  onClose: () => void;
  onDismiss: () => void;
  onOpenProfile: () => void;
}) {
  return (
    <Modal onClose={onClose} title="Зачем подключать TickTick">
      <div className="tickTickWhyDialog">
        <p><strong>TickTick не обязателен.</strong> Все задачи полноценно работают внутри identity workspace и без подключения стороннего сервиса.</p>
        <p><strong>identity workspace — веб-сервис.</strong> У него пока нет собственных виджетов для домашнего экрана телефона.</p>
        <p>У TickTick есть удобные виджеты: задачи можно видеть и отмечать выполненными прямо с главного экрана, не открывая браузер.</p>
        <p>После подключения список будет синхронизироваться в обе стороны: задачи из identity workspace попадут в TickTick, а задачи из TickTick — в identity workspace.</p>
        <p className="integrationHint">Авторизация и управление подключением находятся во вкладке «Профиль».</p>
        <div className="modalActions">
          <button type="button" className="secondaryButton" onClick={onDismiss}>Больше не показывать</button>
          <button type="button" className="primaryButton" onClick={onOpenProfile}>Открыть профиль</button>
        </div>
      </div>
    </Modal>
  );
}

function ProfilePage({
  user,
  profile,
  tickTickStatus,
  trackersEnabled,
  onOpenCard,
  onTickTick,
  onTrackersEnabledChange,
}: {
  user: AuthUser;
  profile: Profile;
  tickTickStatus: TickTickStatus | null;
  trackersEnabled: boolean;
  onOpenCard: () => void;
  onTickTick: () => void;
  onTrackersEnabledChange: (enabled: boolean) => void;
}) {
  const [fatSecretStatus, setFatSecretStatus] = useState<FatSecretStatus | null>(null);
  const [fatSecretBusy, setFatSecretBusy] = useState(false);
  const [fatSecretError, setFatSecretError] = useState<string | null>(null);
  const [fatSecretNotice] = useState<FatSecretNotice | null>(consumeFatSecretCallbackNotice);

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
        <div><span>Карта действует до</span><strong>{profile.expiry ? formatDate(profile.expiry) : "Не указано"}</strong></div>
      </section>

      <section className="profilePreferences" aria-labelledby="profile-preferences-title">
        <header>
          <span className="profileKicker">Интерфейс</span>
          <h2 id="profile-preferences-title">Настройки</h2>
        </header>
        <label className="profileToggle">
          <span><strong>Показывать трекеры</strong><small>Область с трекерами под картой профиля</small></span>
          <input type="checkbox" checked={trackersEnabled} onChange={(event) => onTrackersEnabledChange(event.target.checked)} />
          <i aria-hidden="true" />
        </label>
      </section>

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
          <p>Подключается только существующий аккаунт. Логин и пароль FatSecret не передаются identity workspace.</p>
          {!fatSecretStatus ? null : !fatSecretStatus.configured ? (
            <p className="integrationHint">Добавьте Consumer Key и Consumer Secret FatSecret в серверный <code>.env</code>.</p>
          ) : fatSecretStatus.connected ? (
            <button type="button" className="secondaryButton" disabled={fatSecretBusy} onClick={() => void disconnectFatSecret()}>{fatSecretBusy ? "ОТКЛЮЧЕНИЕ…" : "Отключить FatSecret"}</button>
          ) : (
            <button type="button" className="primaryButton" onClick={connectFatSecret}>Подключить FatSecret</button>
          )}
        </article>

        <article className="profileIntegrationCard">
          <div className="profileIntegrationHead">
            <div>
              <span className="integrationMonogram">TT</span>
              <div><strong>TickTick</strong><small>Двусторонняя синхронизация задач</small></div>
            </div>
            <span className={`profileIntegrationStatus ${tickTickStatus?.connected ? "isConnected" : ""} ${tickTickStatus?.failedTasks ? "hasError" : ""}`}>
              {!tickTickStatus ? "Проверка" : tickTickStatus.connected ? "Подключено" : "Не подключено"}
            </span>
          </div>
          <p>Задачи синхронизируются, пока identity workspace открыт. Управление OAuth-подключением находится здесь, а не во вкладке задач.</p>
          {Boolean(tickTickStatus?.failedTasks) && <p className="integrationNotice integrationNotice-error">Задач с ошибкой синхронизации: {tickTickStatus?.failedTasks}</p>}
          <button type="button" className="primaryButton" onClick={onTickTick}>{tickTickStatus?.connected ? "Управлять TickTick" : "Подключить TickTick"}</button>
        </article>
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
  const [categories, setCategories] = useState<TaskCategory[]>([]);
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
    <Modal onClose={onClose} title="Редактировать задачу">
      <form onSubmit={(event) => {
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
        });
      }}>
        <Field label="Название"><input className="input" value={title} maxLength={160} autoFocus onChange={(event) => setTitle(event.target.value)} /></Field>
        <Field label="Описание"><textarea className="input taskDescriptionInput" value={description} maxLength={2000} rows={4} placeholder="Необязательные подробности" onChange={(event) => setDescription(event.target.value)} /></Field>
        <div className="taskFormGrid">
          <div className="field"><span className="fieldLabel">Категория</span><TaskCategoryPicker value={category} categories={editorCategories} onChange={setCategory} onCreate={createEditorCategory} /></div>
          <div className="field"><span className="fieldLabel">Дата</span><TaskDateControl value={dueDate} currentDate={currentDate} allowUndated={categoryAllowsUndated} onChange={(value) => { setDueDate(value); if (!value) setDueTime(""); }} /></div>
          <div className="field"><span className="fieldLabel">Время задачи</span><TaskTimeControl value={dueTime} disabled={!dueDate} onChange={setDueTime} /></div>
          <Field label="Напомнить"><input className="input" type="datetime-local" value={reminderLocal} onChange={(event) => setReminderLocal(event.target.value)} /></Field>
        </div>
        <Field label="Статус"><select className="input" value={status} onChange={(event) => setStatus(event.target.value as TaskStatus)}><option value="todo">Не выполнена</option><option value="done">Выполнена</option></select></Field>
        <div className="field"><span className="fieldLabel">Приоритет</span><TaskPriorityPicker value={priority} onChange={setPriority} /></div>
        {reminderLocal && notificationState !== "enabled" && <div className="notificationSetup"><div><strong>Уведомления на устройство</strong><span>{notificationState === "denied" ? "Разрешение заблокировано в настройках браузера." : notificationState === "unsupported" ? "Этот браузер или режим не поддерживает Web Push." : "Разрешите уведомления, чтобы напоминание пришло даже при закрытом приложении. На iPhone установите PWA на экран «Домой»."}</span></div>{notificationState !== "denied" && notificationState !== "unsupported" && <button type="button" className="secondaryButton" onClick={() => void registerDeviceNotifications(true).then(setNotificationState)}>Включить</button>}</div>}
        <div className="modalActions"><button type="button" className="secondaryButton" onClick={onClose}>Отмена</button><button className="primaryButton" disabled={!title.trim() || !scheduleIsValid}>Сохранить</button></div>
      </form>
    </Modal>
  );
}

function TaskTimeControl({
  value,
  disabled,
  onChange,
}: {
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <div className="taskTimeControl">
      <input className="input" type="time" value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)} />
      <button type="button" className={!value ? "active" : ""} disabled={disabled} aria-pressed={!value} onClick={() => onChange("")}>Без времени</button>
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
      <input className="input taskDateInput" type="date" value={value} onChange={(event) => onChange(event.target.value)} />
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
  const [deadline, setDeadline] = useState(goal?.deadline ?? "");
  const [related, setRelated] = useState<string[]>(goal?.relatedTaskIds.filter((id) => tasks.some((task) => String(task.id) === id)) ?? []);
  const [completed, setCompleted] = useState(goal?.completed ?? false);
  const [localError, setLocalError] = useState("");

  function submit(event: React.FormEvent) {
    event.preventDefault();
    const currentValue = Number(current);
    const targetValue = Number(target);
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
      <form onSubmit={submit}>
        <Field label="Название"><input className="input" value={title} maxLength={80} autoFocus placeholder="Например: Запустить сервис аналитики" onChange={(e) => setTitle(e.target.value)} /></Field>
        <Field label="Краткое резюме для карты (одно предложение)"><input className="input" value={summary} maxLength={180} placeholder="Что именно было сделано и почему это важно" onChange={(e) => setSummary(e.target.value)} /></Field>
        <Field label="Описание"><textarea className="input textarea" value={description} maxLength={1200} placeholder="Контекст, результат, детали проекта" onChange={(e) => setDescription(e.target.value)} /></Field>
        <div className="formGrid">
          <Field label="Сейчас"><input className="input" type="number" min="0" step="any" value={current} onChange={(e) => setCurrent(e.target.value)} /></Field>
          <Field label="Цель"><input className="input" type="number" min="0.01" step="any" value={target} onChange={(e) => setTarget(e.target.value)} /></Field>
          <Field label="Единица"><input className="input" value={unit} maxLength={16} placeholder="этапов / % / модулей" onChange={(e) => setUnit(e.target.value)} /></Field>
          <Field label="Дедлайн"><input className="input" type="date" value={deadline} onChange={(e) => setDeadline(e.target.value)} /></Field>
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
    expiry: profile.expiry,
  });
  const [localError, setLocalError] = useState("");
  const update = (key: keyof typeof value, next: string) => setValue((current) => ({ ...current, [key]: next }));
  return (
    <Modal onClose={onClose} title="Профиль">
      <form onSubmit={async (e) => {
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
          <Field label="Дата рождения"><input className="input" value={value.dob} maxLength={16} autoFocus={focusField === "dob"} placeholder="15.05.2006" onChange={(e) => update("dob", e.target.value)} /></Field>
          <Field label="Действительно до"><input className="input" value={value.expiry} maxLength={16} autoFocus={focusField === "expiry"} placeholder="15.05.2036" onChange={(e) => update("expiry", e.target.value)} /></Field>
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

function Modal({ onClose, title, wide = false, children }: { onClose: () => void; title: string; wide?: boolean; children: React.ReactNode }) {
  return (
    <div className="overlay" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className={`modal ${wide ? "modalWide" : ""}`}>
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
