import { useCallback, useEffect, useLayoutEffect, useState } from "react";
import type { ThemePreferences } from "../../domain/theme";
import { normalizeTheme, resolveTheme } from "../../application/theme";
import { applyTheme, loadTheme, saveTheme, themeStorageKey } from "../../infrastructure/browser/theme";

export function useTheme(userID: number) {
  const [preferences, setPreferences] = useState(() => normalizeTheme(loadTheme(userID)));

  useLayoutEffect(() => {
    let timer: number | undefined;
    const refresh = () => {
      window.clearTimeout(timer);
      applyTheme(resolveTheme(preferences, new Date()));
      if (preferences.mode === "scheduled") timer = window.setTimeout(refresh, 60_000 - Date.now() % 60_000);
    };
    refresh();
    // Recheck local time after sleep, a clock/timezone change or returning to the PWA.
    window.addEventListener("focus", refresh);
    window.addEventListener("pageshow", refresh);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("focus", refresh);
      window.removeEventListener("pageshow", refresh);
      document.removeEventListener("visibilitychange", refresh);
      applyTheme("light");
    };
  }, [preferences]);

  useEffect(() => {
    const sync = (event: StorageEvent) => {
      if (event.key === themeStorageKey(userID) || event.key === null) setPreferences(normalizeTheme(loadTheme(userID)));
    };
    window.addEventListener("storage", sync);
    return () => window.removeEventListener("storage", sync);
  }, [userID]);

  const changeTheme = useCallback((next: ThemePreferences) => {
    const normalized = normalizeTheme(next);
    saveTheme(userID, normalized);
    setPreferences(normalized);
  }, [userID]);

  return [preferences, changeTheme] as const;
}
