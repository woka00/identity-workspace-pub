import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type {
  TimeActivityStatistics,
  TimeStatistics,
  TimeStatisticsPeriod,
  TimeStatisticsSession,
} from "../../domain/models";
import { api } from "../../infrastructure/http/apiClient";

const PERIODS: Array<{ id: TimeStatisticsPeriod; label: string }> = [
  { id: "today", label: "Сегодня" },
  { id: "week", label: "Неделя" },
  { id: "month", label: "Месяц" },
];

function parseDateKey(value: string) {
  const [year, month, day] = value.split("-").map(Number);
  return new Date(year, month - 1, day, 12);
}

function formatDuration(seconds: number) {
  const safe = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  if (hours > 0) return `${hours}ч ${minutes.toString().padStart(2, "0")}м`;
  if (minutes > 0) return `${minutes}м`;
  return safe > 0 ? `${safe}с` : "0м";
}

function plural(value: number, one: string, few: string, many: string) {
  const lastTwo = Math.abs(value) % 100;
  const last = lastTwo % 10;
  if (lastTwo >= 11 && lastTwo <= 19) return many;
  if (last === 1) return one;
  if (last >= 2 && last <= 4) return few;
  return many;
}

function formatPeriodLabel(statistics: TimeStatistics) {
  if (statistics.period === "today") {
    return parseDateKey(statistics.date).toLocaleDateString("ru-RU", { day: "numeric", month: "long", year: "numeric" });
  }
  if (statistics.period === "week") {
    const start = parseDateKey(statistics.periodStart).toLocaleDateString("ru-RU", { day: "numeric", month: "short" });
    const end = parseDateKey(statistics.periodEnd).toLocaleDateString("ru-RU", { day: "numeric", month: "short" });
    return `${start} — ${end}`;
  }
  const label = parseDateKey(statistics.date).toLocaleDateString("ru-RU", { month: "long", year: "numeric" });
  return label.charAt(0).toUpperCase() + label.slice(1);
}

function formatSummaryCaption(statistics: TimeStatistics) {
  if (statistics.period === "week") return "за последние 7 дней";
  if (statistics.period === "month") {
    const month = parseDateKey(statistics.date).toLocaleDateString("ru-RU", { month: "long" });
    return `за ${month}`;
  }
  return "за сегодня";
}

function sessionTime(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "—";
  return parsed.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
}

function sessionDayTitle(date: string, today: string) {
  const label = parseDateKey(date).toLocaleDateString("ru-RU", { day: "numeric", month: "long" });
  return date === today ? `Сегодня, ${label}` : label;
}

function groupSessions(sessions: TimeStatisticsSession[]) {
  const groups = new Map<string, TimeStatisticsSession[]>();
  sessions.forEach((session) => {
    const current = groups.get(session.date) ?? [];
    current.push(session);
    groups.set(session.date, current);
  });
  return [...groups.entries()];
}

function BackIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m14.5 6-6 6 6 6" /></svg>;
}

function StatisticsSessionHistory({ sessions, today }: { sessions: TimeStatisticsSession[]; today: string }) {
  const groups = useMemo(() => groupSessions(sessions), [sessions]);
  if (groups.length === 0) {
    return <div className="timeStatisticsEmpty"><strong>Сессий пока нет</strong><span>Запуски секундомера появятся здесь.</span></div>;
  }
  return (
    <div className="timeStatisticsSessionGroups">
      {groups.map(([date, entries]) => (
        <section className="timeStatisticsSessionDay" key={date}>
          <h3>{sessionDayTitle(date, today)}</h3>
          <div>
            {entries.map((session, index) => (
              <article className="timeStatisticsSessionRow" key={`${session.id}-${session.date}-${index}`}>
                <span><strong>{session.activityName}</strong><small>{sessionTime(session.startedAt)} — {session.active ? "сейчас" : sessionTime(session.endedAt ?? "")}</small></span>
                <b>{formatDuration(session.durationSeconds)}</b>
              </article>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function WeekBars({ statistics }: { statistics: TimeStatistics }) {
  const maximum = Math.max(1, ...statistics.days.map((day) => day.totalSeconds));
  return (
    <div className="timeStatisticsWeekBars" aria-label="Время по дням за последние семь дней">
      {statistics.days.map((day) => {
        const height = day.totalSeconds === 0 ? 6 : Math.max(12, Math.round(day.totalSeconds / maximum * 100));
        return (
          <div key={day.date} title={`${formatDuration(day.totalSeconds)}`}>
            <span><i style={{ height: `${height}%` }} /></span>
            <small>{parseDateKey(day.date).toLocaleDateString("ru-RU", { weekday: "short" }).replace(".", "")}</small>
          </div>
        );
      })}
    </div>
  );
}

function MonthOverview({ statistics }: { statistics: TimeStatistics }) {
  const date = parseDateKey(statistics.date);
  const daysInMonth = new Date(date.getFullYear(), date.getMonth() + 1, 0).getDate();
  const firstWeekday = (new Date(date.getFullYear(), date.getMonth(), 1).getDay() + 6) % 7;
  const totals = new Map(statistics.days.map((day) => [day.date, day.totalSeconds]));
  const maximum = Math.max(1, ...statistics.days.map((day) => day.totalSeconds));
  const cells = Array.from({ length: firstWeekday + daysInMonth }, (_, index) => {
    if (index < firstWeekday) return null;
    const day = index - firstWeekday + 1;
    const key = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
    return { day, total: totals.get(key) ?? 0, future: key > statistics.date };
  });
  return (
    <div className="timeStatisticsMonth" aria-label="Активность по дням месяца">
      <div className="timeStatisticsMonthWeekdays">{["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"].map((day) => <span key={day}>{day}</span>)}</div>
      <div className="timeStatisticsMonthGrid">
        {cells.map((cell, index) => cell === null ? <span key={`blank-${index}`} /> : (
          <div className={cell.future ? "isFuture" : ""} key={cell.day} title={formatDuration(cell.total)}>
            <small>{cell.day}</small>
            <i style={{ opacity: cell.total === 0 ? .08 : .25 + cell.total / maximum * .75 }} />
          </div>
        ))}
      </div>
    </div>
  );
}

function ActivityDetails({ value, today, loading }: { value: TimeActivityStatistics | null; today: string; loading: boolean }) {
  if (loading) return <div className="timeStatisticsLoading" role="status">Загружаем активность…</div>;
  if (!value) return null;
  return (
    <>
      <section className="timeActivityStatisticsHero">
        <span>Всего</span>
        <strong>{formatDuration(value.totalSeconds)}</strong>
        <div>
          <p><b>{formatDuration(value.weekSeconds)}</b><small>за последние 7 дней</small></p>
          <p><b>{value.sessionCount}</b><small>{plural(value.sessionCount, "сессия", "сессии", "сессий")}</small></p>
          <p><b>{formatDuration(value.averageSessionSeconds)}</b><small>средняя сессия</small></p>
        </div>
      </section>
      <section className="timeStatisticsSection">
        <header><span>История</span><h2>Все сессии</h2></header>
        <StatisticsSessionHistory sessions={value.sessions} today={today} />
      </section>
    </>
  );
}

export default function TimeStatisticsPage({ onBack }: { onBack: () => void }) {
  const [period, setPeriod] = useState<TimeStatisticsPeriod>("today");
  const [statistics, setStatistics] = useState<TimeStatistics | null>(null);
  const [activity, setActivity] = useState<TimeActivityStatistics | null>(null);
  const [activityID, setActivityID] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [activityLoading, setActivityLoading] = useState(false);
  const [error, setError] = useState("");
  const requestSequence = useRef(0);

  const load = useCallback(async (selectedPeriod: TimeStatisticsPeriod, quiet = false) => {
    const sequence = ++requestSequence.current;
    if (!quiet) setLoading(true);
    try {
      const value = await api.timeStatistics(selectedPeriod);
      if (sequence !== requestSequence.current) return;
      setStatistics(value);
      setError("");
    } catch (cause) {
      if (sequence !== requestSequence.current) return;
      setError(cause instanceof Error ? cause.message : "Не удалось загрузить статистику");
    } finally {
      if (sequence === requestSequence.current && !quiet) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load(period);
  }, [load, period]);

  useEffect(() => {
    const hasActiveSession = statistics?.sessions.some((session) => session.active);
    if (!hasActiveSession || activityID !== null) return;
    const timer = window.setInterval(() => void load(period, true), 30_000);
    return () => window.clearInterval(timer);
  }, [activityID, load, period, statistics?.sessions]);

  async function openActivity(id: number) {
    setActivityID(id);
    setActivity(null);
    setActivityLoading(true);
    setError("");
    try {
      setActivity(await api.timeActivityStatistics(id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось загрузить активность");
    } finally {
      setActivityLoading(false);
    }
  }

  function goBack() {
    if (activityID !== null) {
      setActivityID(null);
      setActivity(null);
      setError("");
      return;
    }
    onBack();
  }

  const sessionToday = statistics?.date ?? new Date().toISOString().slice(0, 10);
  const activityName = activity?.name ?? statistics?.activities.find((item) => item.id === activityID)?.name;

  return (
    <section className="timeStatisticsPage" aria-labelledby="time-statistics-title">
      <header className="timeStatisticsHeader">
        <button type="button" onClick={goBack} aria-label="Назад"><BackIcon /></button>
        <div>
          <span>{activityID === null ? "Время" : "Статистика активности"}</span>
          <h1 id="time-statistics-title">{activityID === null ? "Статистика" : activityName ?? "Активность"}</h1>
          <p>{activityID === null && statistics ? formatPeriodLabel(statistics) : "Подробная история времени"}</p>
        </div>
        {activityID === null && (
          <div className="timeStatisticsPeriods" role="tablist" aria-label="Период статистики">
            {PERIODS.map((item) => <button type="button" role="tab" aria-selected={period === item.id} className={period === item.id ? "isActive" : ""} onClick={() => setPeriod(item.id)} key={item.id}>{item.label}</button>)}
          </div>
        )}
      </header>

      {error && <div className="timeTrackerError" role="alert">{error}</div>}

      {activityID !== null ? (
        <ActivityDetails value={activity} today={sessionToday} loading={activityLoading} />
      ) : loading && !statistics ? (
        <div className="timeStatisticsLoading" role="status">Собираем статистику…</div>
      ) : statistics ? (
        <>
          <section className="timeStatisticsHero">
            <span>Общее время</span>
            <strong>{formatDuration(statistics.totalSeconds)}</strong>
            <small>{formatSummaryCaption(statistics)}</small>
            <div>
              {statistics.period === "month" && <p><b>{statistics.activeDays}</b><span>{plural(statistics.activeDays, "активный день", "активных дня", "активных дней")}</span></p>}
              <p><b>{statistics.activityCount}</b><span>{plural(statistics.activityCount, "активность", "активности", "активностей")}</span></p>
              <p><b>{statistics.sessionCount}</b><span>{plural(statistics.sessionCount, "сессия", "сессии", "сессий")}</span></p>
              {statistics.longestSessionSeconds > 0 && <p><b>{formatDuration(statistics.longestSessionSeconds)}</b><span>самая длинная</span></p>}
            </div>
            {statistics.period === "week" && <WeekBars statistics={statistics} />}
            {statistics.period === "month" && <MonthOverview statistics={statistics} />}
          </section>

          <section className="timeStatisticsSection">
            <header><span>Распределение</span><h2>Активности</h2></header>
            {statistics.activities.length > 0 ? (
              <div className="timeStatisticsActivities">
                {statistics.activities.map((item, index) => (
                  <button type="button" onClick={() => void openActivity(item.id)} key={item.id}>
                    <span><strong>{item.name}</strong><small>{formatDuration(item.totalSeconds)} · {item.sharePercent}%</small></span>
                    <i><b style={{ width: `${item.sharePercent}%`, opacity: Math.max(.38, 1 - index * .12) }} /></i>
                    <em aria-hidden="true">›</em>
                  </button>
                ))}
              </div>
            ) : <div className="timeStatisticsEmpty"><strong>Пока нет данных</strong><span>Запустите любую задачу, чтобы увидеть распределение времени.</span></div>}
          </section>

          <section className="timeStatisticsSection">
            <header><span>Подробно</span><h2>Сессии</h2></header>
            <StatisticsSessionHistory sessions={statistics.sessions} today={statistics.date} />
          </section>
        </>
      ) : null}
    </section>
  );
}
