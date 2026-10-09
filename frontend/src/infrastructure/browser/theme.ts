import type { AppliedTheme, ThemePreferences } from "../../domain/theme";

export function themeStorageKey(userID: number) {
  return `identity-workspace.theme.v1.user-${userID}`;
}

export function loadTheme(userID: number): unknown {
  try { return JSON.parse(window.localStorage.getItem(themeStorageKey(userID)) ?? "null"); }
  catch { return null; }
}

export function saveTheme(userID: number, preferences: ThemePreferences) {
  try { window.localStorage.setItem(themeStorageKey(userID), JSON.stringify(preferences)); }
  catch { /* Keep the appearance choice for this session if storage is unavailable. */ }
}

const THEME_META: Record<AppliedTheme, { themeColor: string; colorScheme: "light" | "dark" }> = {
  light: { themeColor: "#f5f5f2", colorScheme: "light" },
  dark: { themeColor: "#070709", colorScheme: "dark" },
  apple: { themeColor: "#f5f5f7", colorScheme: "light" },
};

export function applyTheme(theme: AppliedTheme) {
  if (document.documentElement.dataset.theme === theme) return;
  document.documentElement.dataset.theme = theme;
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", THEME_META[theme].themeColor);
  document.querySelector('meta[name="color-scheme"]')?.setAttribute("content", THEME_META[theme].colorScheme);
}
