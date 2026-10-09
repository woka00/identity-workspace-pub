const FORMAL_DAY_START_HOUR = 3;

export function parseDateKey(value: string) {
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

export function dateKeyFromDate(value: Date) {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function localTodayKey() {
  return dateKeyFromDate(new Date());
}

export function formalTodayKey(calendarDate = localTodayKey()) {
  return new Date().getHours() < FORMAL_DAY_START_HOUR ? addDaysToDateKey(calendarDate, -1) : calendarDate;
}

export function dateKeyFromTimestamp(value: string) {
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

export function addDaysToDateKey(value: string, amount: number) {
  const parsed = parseDateKey(value);
  if (!parsed) return value;
  parsed.setDate(parsed.getDate() + amount);
  return dateKeyFromDate(parsed);
}

export function dateKeysInRange(from: string, to: string, maximumDays: number) {
  if (!parseDateKey(from) || !parseDateKey(to) || from > to) return null;
  const dates: string[] = [];
  let cursor = from;
  while (cursor <= to && dates.length <= maximumDays) {
    dates.push(cursor);
    cursor = addDaysToDateKey(cursor, 1);
  }
  return dates.length > maximumDays ? null : dates;
}

export function startOfWeekDateKey(value: string) {
  const parsed = parseDateKey(value) ?? new Date();
  parsed.setHours(12, 0, 0, 0);
  const daysSinceMonday = (parsed.getDay() + 6) % 7;
  parsed.setDate(parsed.getDate() - daysSinceMonday);
  return dateKeyFromDate(parsed);
}

export function startOfMonthDateKey(value: string) {
  const parsed = parseDateKey(value) ?? new Date();
  parsed.setDate(1);
  parsed.setHours(12, 0, 0, 0);
  return dateKeyFromDate(parsed);
}

export function addMonthsToDateKey(value: string, amount: number) {
  const parsed = parseDateKey(value) ?? new Date();
  parsed.setDate(1);
  parsed.setMonth(parsed.getMonth() + amount);
  return dateKeyFromDate(parsed);
}
