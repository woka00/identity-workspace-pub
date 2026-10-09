import type { AppliedTheme, ThemePreferences } from "../domain/theme";

export const DEFAULT_THEME: ThemePreferences = { mode: "light", darkStart: "22:00", darkEnd: "07:00" };

export function validThemeTime(value: unknown): value is string {
  return typeof value === "string" && /^([01]\d|2[0-3]):[0-5]\d$/.test(value);
}

export function normalizeTheme(value: unknown): ThemePreferences {
  const input = value && typeof value === "object" ? value as Partial<ThemePreferences> : {};
  const validSchedule = validThemeTime(input.darkStart) && validThemeTime(input.darkEnd) && input.darkStart !== input.darkEnd;
  return {
    mode: input.mode === "dark" || input.mode === "apple" || input.mode === "scheduled" ? input.mode : "light",
    darkStart: validSchedule ? input.darkStart! : DEFAULT_THEME.darkStart,
    darkEnd: validSchedule ? input.darkEnd! : DEFAULT_THEME.darkEnd,
  };
}

export function resolveTheme(preferences: ThemePreferences, now: Date): AppliedTheme {
  if (preferences.mode !== "scheduled") return preferences.mode;
  return isScheduledDark(preferences, now) ? "dark" : "light";
}

export function isDarkTheme(preferences: ThemePreferences, now: Date): boolean {
  return resolveTheme(preferences, now) === "dark";
}

function isScheduledDark(preferences: ThemePreferences, now: Date): boolean {
  const time = `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
  const { darkStart, darkEnd } = preferences;
  // Start is inclusive, end exclusive; overnight schedules cross midnight.
  return darkStart < darkEnd
    ? time >= darkStart && time < darkEnd
    : time >= darkStart || time < darkEnd;
}
