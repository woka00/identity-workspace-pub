import { useEffect, useState } from "react";
import type { ThemeMode, ThemePreferences } from "../../domain/theme";
import { validThemeTime } from "../../application/theme";
import "./themeSettings.css";

const MODES: Array<{ mode: ThemeMode; label: string }> = [
  { mode: "light", label: "Светлый" },
  { mode: "dark", label: "Чёрный" },
  { mode: "apple", label: "Apple" },
  { mode: "scheduled", label: "По расписанию" },
];

export default function ThemeSettings({ preferences, onChange }: {
  preferences: ThemePreferences;
  onChange: (preferences: ThemePreferences) => void;
}) {
  const [start, setStart] = useState(preferences.darkStart);
  const [end, setEnd] = useState(preferences.darkEnd);
  useEffect(() => { setStart(preferences.darkStart); setEnd(preferences.darkEnd); }, [preferences.darkStart, preferences.darkEnd]);
  const valid = validThemeTime(start) && validThemeTime(end) && start !== end;
  const changed = start !== preferences.darkStart || end !== preferences.darkEnd;

  return (
    <section className="profileTheme" aria-labelledby="profile-theme-title">
      <div className="profileThemeHeading"><strong id="profile-theme-title">Цветовая тема</strong><small>Настройка сохраняется для вашего профиля в этом браузере.</small></div>
      <div className="profileThemeModes" role="group" aria-label="Цветовая тема">
        {MODES.map(({ mode, label }) => (
          <button type="button" key={mode} data-theme-mode={mode} aria-pressed={preferences.mode === mode} onClick={() => onChange({ ...preferences, mode })}>
            <span className="profileThemeModeSwatch" aria-hidden="true" />
            <span>{label}</span>
          </button>
        ))}
      </div>
      {preferences.mode === "scheduled" && (
        <form className="profileThemeSchedule" noValidate onSubmit={(event) => { event.preventDefault(); if (valid) onChange({ ...preferences, darkStart: start, darkEnd: end }); }}>
          <p>Чёрная тема с {preferences.darkStart} до {preferences.darkEnd}, в остальное время — светлая. Используется местное время устройства.</p>
          <div className="profileThemeTimes">
            <label className="field"><span className="fieldLabel">Включать чёрную</span><input className="input" type="time" required step="60" value={start} onChange={(event) => setStart(event.target.value)} /></label>
            <label className="field"><span className="fieldLabel">Включать светлую</span><input className="input" type="time" required step="60" value={end} onChange={(event) => setEnd(event.target.value)} /></label>
          </div>
          {!valid && <p className="formError" role="alert">Укажите разное время начала и окончания.</p>}
          <button type="submit" className="secondaryButton" disabled={!valid || !changed}>Сохранить расписание</button>
        </form>
      )}
    </section>
  );
}
