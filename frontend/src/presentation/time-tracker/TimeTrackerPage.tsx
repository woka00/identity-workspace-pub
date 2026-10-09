import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { NotificationConfig, TimeActivity, TimeTrackerPeriod, TimeTrackerState } from "../../domain/models";
import {
  currentDeviceNotificationState,
  registerDeviceNotifications,
} from "../../infrastructure/browser/deviceNotifications";
import type { DeviceNotificationState } from "../../infrastructure/browser/deviceNotifications";
import { api } from "../../infrastructure/http/apiClient";
import { useVirtualKeyboardOpen } from "../hooks/useVirtualKeyboardOpen";
import TimeStatisticsPage from "./TimeStatisticsPage";

const ACTIVITY_SUGGESTIONS = ["Дизайн системы", "Разработка", "Английский", "Спорт", "Чтение"];
const DEFAULT_REMINDER_MESSAGE = "Пора сделать перерыв и немного подвигаться";

function formatTrackerDate(value: string) {
  const [year, month, day] = value.split("-").map(Number);
  const parsed = new Date(year, month - 1, day, 12);
  if (Number.isNaN(parsed.getTime())) return "Сегодня";
  const formatted = parsed.toLocaleDateString("ru-RU", { day: "numeric", month: "long" });
  return `Сегодня, ${formatted}`;
}

function localDateKey() {
  const now = new Date();
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function previousDateKey(value: string) {
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(year, month - 1, day, 12);
  date.setDate(date.getDate() - 1);
  const previousYear = date.getFullYear();
  const previousMonth = String(date.getMonth() + 1).padStart(2, "0");
  const previousDay = String(date.getDate()).padStart(2, "0");
  return `${previousYear}-${previousMonth}-${previousDay}`;
}

function formatLastStartedDate(value: string | undefined, today: string) {
  if (!value) return "Ещё не запускалась";
  if (value === today) return "Последний запуск · Сегодня";
  if (value === previousDateKey(today)) return "Последний запуск · Вчера";
  const [, month, day] = value.split("-");
  return `Последний запуск · ${day}.${month}`;
}

function formatDuration(seconds: number) {
  const safe = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  if (hours > 0) return `${hours}ч ${minutes.toString().padStart(2, "0")}м`;
  if (minutes > 0) return `${minutes}м`;
  return safe > 0 ? `${safe}с` : "0м";
}

function timerParts(seconds: number) {
  const safe = Math.max(0, Math.floor(seconds));
  return {
    hours: Math.floor(safe / 3600).toString().padStart(2, "0"),
    minutes: Math.floor((safe % 3600) / 60).toString().padStart(2, "0"),
    seconds: Math.floor(safe % 60).toString().padStart(2, "0"),
  };
}

function PlayIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9 7 8 5-8 5Z" /></svg>;
}

function PauseIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 7v10M15 7v10" /></svg>;
}

function StopIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="7" y="7" width="10" height="10" rx="2" /></svg>;
}

function ClockIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8.5" /><path d="M12 7.5V12l3 2" /></svg>;
}

export default function TimeTrackerPage({ onOpenCalories, onOpenIntroduction }: { onOpenCalories?: () => void; onOpenIntroduction: () => void }) {
  const [period, setPeriod] = useState<TimeTrackerPeriod>("today");
  const [state, setState] = useState<TimeTrackerState | null>(null);
  const [anchor, setAnchor] = useState(() => Date.now());
  const [now, setNow] = useState(() => Date.now());
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<number | "stop" | "save" | "delete" | null>(null);
  const [error, setError] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [editingActivity, setEditingActivity] = useState<TimeActivity | null>(null);
  const [statisticsOpen, setStatisticsOpen] = useState(false);
  const [activityName, setActivityName] = useState("");
  const [reminderEnabled, setReminderEnabled] = useState(false);
  const [reminderInterval, setReminderInterval] = useState("30");
  const [reminderMessage, setReminderMessage] = useState("");
  const [notificationConfig, setNotificationConfig] = useState<NotificationConfig | null>(null);
  const [notificationState, setNotificationState] = useState<DeviceNotificationState>(() => {
    const current = currentDeviceNotificationState();
    return current === "enabled" ? "unknown" : current;
  });
  const keyboardOpen = useVirtualKeyboardOpen();
  const requestSequence = useRef(0);

  const applyState = useCallback((value: TimeTrackerState) => {
    setState(value);
    const timestamp = Date.now();
    setAnchor(timestamp);
    setNow(timestamp);
  }, []);

  const load = useCallback(async (selectedPeriod: TimeTrackerPeriod, quiet = false) => {
    const sequence = ++requestSequence.current;
    if (!quiet) setLoading(true);
    try {
      const value = await api.timeTracker(selectedPeriod);
      if (sequence !== requestSequence.current) return;
      applyState(value);
      setError("");
    } catch (cause) {
      if (sequence !== requestSequence.current) return;
      setError(cause instanceof Error ? cause.message : "Не удалось загрузить трекер");
    } finally {
      if (sequence === requestSequence.current && !quiet) setLoading(false);
    }
  }, [applyState]);

  useEffect(() => {
    void load(period);
  }, [load, period]);

  useEffect(() => {
    let active = true;
    api.notificationConfig()
      .then(async (config) => {
        if (!active) return;
        setNotificationConfig(config);
        const current = currentDeviceNotificationState();
        if (!config.configured || current === "unsupported") {
          setNotificationState("unsupported");
        } else if (current === "enabled") {
          const synced = await registerDeviceNotifications(false, config);
          if (active) setNotificationState(synced);
        } else {
          setNotificationState(current);
        }
      })
      .catch(() => { if (active) setNotificationState("error"); });
    return () => { active = false; };
  }, []); // A browser permission alone is not enough: confirm the server subscription too.

  useEffect(() => {
    if (!state?.activeSession) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [state?.activeSession]);

  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === "visible") void load(period, true);
    };
    const timer = state?.activeSession ? window.setInterval(refresh, 30_000) : undefined;
    document.addEventListener("visibilitychange", refresh);
    window.addEventListener("focus", refresh);
    return () => {
      if (timer !== undefined) window.clearInterval(timer);
      document.removeEventListener("visibilitychange", refresh);
      window.removeEventListener("focus", refresh);
    };
  }, [load, period, state?.activeSession]);

  const liveDelta = state?.activeSession ? Math.max(0, Math.floor((now - anchor) / 1000)) : 0;
  const selectedSession = state?.activeSession ?? state?.pausedSession;
  const paused = Boolean(state?.pausedSession && !state?.activeSession);
  const activeElapsed = (selectedSession?.elapsedSeconds ?? 0) + liveDelta;
  const todayTotal = (state?.todayTotalSeconds ?? 0) + liveDelta;
  const periodTotal = (state?.periodTotalSeconds ?? 0) + liveDelta;
  const activeTimer = timerParts(activeElapsed);
  const activeActivityID = state?.activeSession?.activityId;

  const activities = useMemo(() => (state?.activities ?? []).map((activity) => ({
    ...activity,
    todaySeconds: activity.todaySeconds + (activity.id === activeActivityID ? liveDelta : 0),
    periodSeconds: activity.periodSeconds + (activity.id === activeActivityID ? liveDelta : 0),
  })), [activeActivityID, liveDelta, state?.activities]);
  const todayActivities = activities.filter((activity) => activity.lastStartedDate === state?.date || activity.id === activeActivityID);

  async function start(activity: Pick<TimeActivity, "id">) {
    if (busy !== null || activity.id === activeActivityID) return;
    setBusy(activity.id);
    try {
      applyState(await api.startTimeActivity(activity.id, period));
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось запустить задачу");
    } finally {
      setBusy(null);
    }
  }

  async function stop(mode: "pause" | "finish") {
    if (busy !== null) return;
    setBusy("stop");
    try {
      const value = mode === "pause" ? await api.pauseTimeActivity(period) : await api.finishTimeActivity(period);
      applyState(value);
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось остановить задачу");
    } finally {
      setBusy(null);
    }
  }

  function openCreateActivity() {
    setEditingActivity(null);
    setActivityName("");
    setReminderEnabled(false);
    setReminderInterval("30");
    setReminderMessage("");
    setError("");
    setCreateOpen(true);
  }

  function openEditActivity(activity: TimeActivity) {
    setEditingActivity(activity);
    setActivityName(activity.name);
    setReminderEnabled(activity.reminderIntervalMinutes > 0);
    setReminderInterval(String(activity.reminderIntervalMinutes || 30));
    setReminderMessage(activity.reminderMessage === DEFAULT_REMINDER_MESSAGE ? "" : activity.reminderMessage || "");
    setError("");
    setCreateOpen(true);
  }

  async function saveActivity(event: React.FormEvent) {
    event.preventDefault();
    const name = activityName.trim();
    if (!name || busy !== null) return;
    const interval = reminderEnabled ? Number(reminderInterval) : 0;
    if (reminderEnabled && (!Number.isInteger(interval) || interval < 5 || interval > 240)) {
      setError("Укажите интервал от 5 до 240 минут");
      return;
    }
    const input = {
      name,
      reminderIntervalMinutes: interval,
      reminderMessage: reminderEnabled ? reminderMessage.trim() : "",
    };
    setBusy("save");
    try {
      if (editingActivity) await api.updateTimeActivity(editingActivity.id, input);
      else await api.createTimeActivity(input);
      setActivityName("");
      setCreateOpen(false);
      await load(period, true);
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось сохранить задачу");
    } finally {
      setBusy(null);
    }
  }

  async function deleteActivity() {
    if (!editingActivity || busy !== null) return;
    if (!window.confirm(`Удалить таймер «${editingActivity.name}»? Накопленная статистика сохранится.`)) return;
    setBusy("delete");
    try {
      await api.deleteTimeActivity(editingActivity.id);
      setCreateOpen(false);
      setEditingActivity(null);
      await load(period, true);
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось удалить таймер");
    } finally {
      setBusy(null);
    }
  }

  if (statisticsOpen) {
    return <TimeStatisticsPage onBack={() => setStatisticsOpen(false)} />;
  }

  return (
    <section className="timeTrackerPage" aria-labelledby="time-tracker-title">
      <header className="timeTrackerHeader">
        <div className="timeTrackerHeading">
          <span>Трекер</span>
          <h1 id="time-tracker-title">Время</h1>
          <p>{formatTrackerDate(state?.date ?? localDateKey())}</p>
        </div>
        <div className="timeTrackerHeaderActions">
          <div className="timePeriodControl" role="tablist" aria-label="Период статистики">
            <button type="button" role="tab" aria-selected={period === "today"} className={period === "today" ? "isActive" : ""} onClick={() => setPeriod("today")}>Сегодня</button>
            <button type="button" role="tab" aria-selected={period === "week"} className={period === "week" ? "isActive" : ""} onClick={() => setPeriod("week")}>Неделя</button>
          </div>
          <button type="button" className="timeCaloriesLink" onClick={onOpenIntroduction}>Как пользоваться <span aria-hidden="true">?</span></button>
          {onOpenCalories && <button type="button" className="timeCaloriesLink" onClick={onOpenCalories}>Калории <span aria-hidden="true">→</span></button>}
        </div>
      </header>

      {selectedSession && (
        <section className={`timeSessionHero${paused ? " isPaused" : ""}`} aria-live="polite" aria-label={`${paused ? "На паузе" : "В работе"}: ${selectedSession.activityName}`}>
          <div className="timeSessionStatus"><i />{paused ? "На паузе" : "В работе"}</div>
          <h2>{selectedSession.activityName}</h2>
          <p>Прошло с начала сессии</p>
          <div className="timeSessionClock" aria-label={`${activeTimer.hours} часов ${activeTimer.minutes} минут ${activeTimer.seconds} секунд`}>
            <div><strong>{activeTimer.hours}</strong><span>часы</span></div>
            <b aria-hidden="true">:</b>
            <div><strong>{activeTimer.minutes}</strong><span>минуты</span></div>
            <b aria-hidden="true">:</b>
            <div><strong>{activeTimer.seconds}</strong><span>секунды</span></div>
          </div>
          <div className="timeSessionActions">
            <button type="button" className="timePauseButton" disabled={busy !== null} onClick={() => paused ? void start({ id: selectedSession.activityId }) : void stop("pause")}>{paused ? <PlayIcon /> : <PauseIcon />}<span>{paused ? "Продолжить" : "Пауза"}</span></button>
            <button type="button" className="timeFinishButton" disabled={busy !== null} onClick={() => void stop("finish")}><StopIcon /><span>Завершить</span></button>
          </div>
        </section>
      )}

      {state && activities.length > 0 && (
        <button type="button" className="timeTodaySummary" aria-label="Открыть подробную статистику времени" onClick={() => setStatisticsOpen(true)}>
          <header>
            <span className="timeTodayIcon"><ClockIcon /></span>
            <div><strong>Сегодня</strong><small>Открыть статистику</small></div>
            <div className="timeTodayTotal"><strong>{formatDuration(todayTotal)}</strong><small>общее время</small></div>
          </header>
          {todayActivities.length > 0 ? (
            <div className="timeTodayActivityList">
              {todayActivities.map((activity) => (
                <div className={activity.id === activeActivityID ? "isActive" : ""} key={activity.id}>
                  <span>{activity.name}{activity.id === activeActivityID && <small><i />В процессе</small>}</span>
                  <strong>{formatDuration(activity.todaySeconds)}</strong>
                </div>
              ))}
            </div>
          ) : <p className="timeTodayEmpty">Сегодня трекер ещё не запускался</p>}
        </button>
      )}

      <section className="timeActivitiesSection" aria-labelledby="time-activities-title">
        <header><div><span>Активности</span><h2 id="time-activities-title">Мои задачи</h2></div>{period === "week" && <small>{formatDuration(periodTotal)} за неделю</small>}</header>
        {loading && !state ? (
          <div className="timeTrackerLoading" role="status">Загружаем задачи…</div>
        ) : activities.length === 0 ? (
          <div className="timeTrackerEmpty">
            <span><ClockIcon /></span>
            <h3>Добавьте первую задачу</h3>
            <p>Создайте активность и запускайте секундомер одним нажатием.</p>
          </div>
        ) : (
          <div className="timeActivityList">
            {activities.map((activity) => {
              const active = activity.id === activeActivityID;
              const activityPaused = paused && activity.id === selectedSession?.activityId;
              return (
                <article className={`timeActivityRow${active ? " isActive" : ""}`} key={activity.id}>
                  <span className="timeActivityMarker" aria-hidden="true"><i /></span>
                  <div className="timeActivityCopy">
                    <strong>{activity.name}</strong>
                    <small>{activityPaused ? "На паузе · " : ""}{formatLastStartedDate(activity.lastStartedDate, state?.date ?? localDateKey())}{activity.reminderIntervalMinutes > 0 ? ` · Напоминание каждые ${activity.reminderIntervalMinutes} мин` : ""}</small>
                  </div>
                  <div className="timeActivityMeta">
                    <button type="button" className="timeActivitySettings" onClick={() => openEditActivity(activity)} aria-label={`Изменить ${activity.name}`}>•••</button>
                  </div>
                  <button
                    type="button"
                    className={`timeActivityControl${active ? " isPause" : ""}`}
                    disabled={busy !== null}
                    aria-label={active ? `Поставить ${activity.name} на паузу` : `${activityPaused ? "Продолжить" : "Запустить"} ${activity.name}`}
                    onClick={() => active ? void stop("pause") : void start(activity)}
                  >
                    {active ? <PauseIcon /> : <PlayIcon />}
                  </button>
                </article>
              );
            })}
          </div>
        )}
        <button type="button" className="timeAddActivity" onClick={openCreateActivity}><span aria-hidden="true">＋</span>Задача</button>
        {error && !createOpen && <div className="timeTrackerError" role="alert">{error}</div>}
      </section>

      {createOpen && (
        <div className="timeActivityOverlay" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && busy === null) setCreateOpen(false); }}>
          <form className={`timeActivityModal${keyboardOpen ? " keyboardOpen" : ""}`} noValidate onSubmit={saveActivity} role="dialog" aria-modal="true" aria-labelledby="time-activity-editor-title">
            <div className="timeActivityModalHandle" aria-hidden="true" />
            <header><div><span>{editingActivity ? "Настройки активности" : "Новая активность"}</span><h2 id="time-activity-editor-title">{editingActivity ? "Изменить задачу" : "Добавить задачу"}</h2></div><button type="button" onClick={() => setCreateOpen(false)} aria-label="Закрыть">×</button></header>
            <label><span>Название</span><input value={activityName} maxLength={60} placeholder="Например, Разработка" onChange={(event) => setActivityName(event.target.value)} /></label>
            {!editingActivity && <div className="timeActivitySuggestions" aria-label="Примеры задач">
              {ACTIVITY_SUGGESTIONS.map((suggestion) => <button type="button" key={suggestion} onClick={() => setActivityName(suggestion)}>{suggestion}</button>)}
            </div>}
            <section className="timeReminderRule" aria-labelledby="time-reminder-rule-title">
              <label className="timeReminderToggle">
                <input type="checkbox" checked={reminderEnabled} onChange={(event) => setReminderEnabled(event.target.checked)} />
                <span><strong id="time-reminder-rule-title">Напоминать во время работы</strong><small>Push-уведомление через заданный интервал</small></span>
              </label>
              {reminderEnabled && <div className="timeReminderFields">
                <label><span>Каждые</span><div><input type="number" min={5} max={240} step={5} inputMode="numeric" value={reminderInterval} onChange={(event) => setReminderInterval(event.target.value)} /><em>минут</em></div></label>
                <label><span>Текст уведомления</span><textarea className="resize-none" maxLength={160} rows={3} value={reminderMessage} placeholder={DEFAULT_REMINDER_MESSAGE} onChange={(event) => setReminderMessage(event.target.value)} /></label>
                {notificationState !== "enabled" && <div className="timeReminderPermission">
                  <span>{notificationState === "denied" ? "Уведомления заблокированы в настройках браузера." : notificationState === "unsupported" && notificationConfig?.configured === false ? "Push-уведомления не настроены на сервере." : notificationState === "unsupported" ? "Web Push недоступен. На iPhone установите приложение на экран «Домой»." : notificationState === "error" ? "Не удалось подключить уведомления. Попробуйте ещё раз." : "Разрешите уведомления на этом устройстве."}</span>
                  {notificationState !== "denied" && notificationState !== "unsupported" && <button type="button" onClick={() => void registerDeviceNotifications(true, notificationConfig ?? undefined).then(setNotificationState)}>Включить</button>}
                </div>}
              </div>}
            </section>
            {editingActivity && <button type="button" className="timeActivityDelete" disabled={busy !== null} onClick={() => void deleteActivity()}>{busy === "delete" ? "Удаляем…" : "Удалить таймер"}</button>}
            <p>{editingActivity ? "Нажмите на название задачи в списке, чтобы снова открыть эти настройки." : "После создания задача появится в списке. Секундомер запускается отдельной кнопкой Play."}</p>
            {error && <div className="timeTrackerError" role="alert">{error}</div>}
            <div className="timeActivityModalActions"><button type="button" onClick={() => setCreateOpen(false)}>Отмена</button><button disabled={!activityName.trim() || busy !== null}>{busy === "save" ? "Сохраняем…" : editingActivity ? "Сохранить" : "Добавить"}</button></div>
          </form>
        </div>
      )}
    </section>
  );
}
