export type ThemeMode = "light" | "dark" | "apple" | "scheduled";
export type AppliedTheme = Exclude<ThemeMode, "scheduled">;

export interface ThemePreferences {
  mode: ThemeMode;
  darkStart: string;
  darkEnd: string;
}
