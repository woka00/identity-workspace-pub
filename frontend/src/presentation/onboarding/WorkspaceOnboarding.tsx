import { useEffect, useState } from "react";
import {
  normalizeWorkspaceNavigation,
  requiredWorkspaceNavigation,
  WORKSPACE_FEATURES,
  workspaceNavigationCandidates,
} from "../../application/workspace";
import type { BottomNavigationItem, WorkspaceFeature } from "../../domain/models";
import IntroductionDialog from "./IntroductionDialog";

export function FeatureIcon({ feature }: { feature: WorkspaceFeature }) {
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    {feature === "tracker" ? <><circle cx="12" cy="13" r="8" /><path d="M9 2h6M12 5V3m0 10 3-3" /></>
      : feature === "calories" ? <path d="M13 3c1 5-5 5-3 9 1-1 2-2 2-4 5 3 7 6 5 10-2 4-9 4-11 0C3 13 7 10 8 7c0 3 1 3 2 3C9 6 12 6 13 3Z" />
      : feature === "tasks" ? <><rect x="4" y="3" width="16" height="18" rx="4" /><path d="m8 9 1 1 2-2m2 1h3m-8 6 1 1 2-2m2 1h3" /></>
      : feature === "projects" ? <><path d="M3 8a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v9H3Z" /><path d="m8 14 3 3 5-5" /></>
      : <><rect x="3" y="3" width="7" height="7" rx="2" /><rect x="14" y="3" width="7" height="7" rx="2" /><rect x="3" y="14" width="7" height="7" rx="2" /><path d="M14 18h7m-3.5-3.5v7" /></>}
  </svg>;
}

const NAVIGATION_LABELS: Record<BottomNavigationItem, string> = {
  card: "Карта", tasks: "Задачи", projects: "Проекты", tracker: "Трекер", calories: "Калории", profile: "Профиль",
};

function NavigationIcon({ item }: { item: BottomNavigationItem }) {
  if (item !== "card" && item !== "profile") return <FeatureIcon feature={item} />;
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    {item === "card"
      ? <><rect x="3" y="5" width="18" height="14" rx="3" /><circle cx="8" cy="11" r="2" /><path d="M13 10h5m-5 4h4" /></>
      : <><circle cx="12" cy="8" r="4" /><path d="M5 21c.7-4.3 3-6.5 7-6.5s6.3 2.2 7 6.5" /></>}
  </svg>;
}

export default function WorkspaceOnboarding({ initialFeatures, initialNavigation, editing, onSave, onClose }: {
  initialFeatures: WorkspaceFeature[];
  initialNavigation: BottomNavigationItem[];
  editing: boolean;
  onSave: (features: WorkspaceFeature[], navigation: BottomNavigationItem[]) => Promise<void>;
  onClose: () => void;
}) {
  const [selected, setSelected] = useState(initialFeatures);
  const [navigation, setNavigation] = useState(() => normalizeWorkspaceNavigation(initialFeatures, initialNavigation));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const heading = document.getElementById("workspace-intro-title");
    heading?.closest("dialog")?.scrollTo({ top: 0 });
    heading?.focus({ preventScroll: true });
  }, []);

  function selectFeature(feature: WorkspaceFeature, checked: boolean) {
    const next = checked ? [...selected, feature] : selected.filter((item) => item !== feature);
    setSelected(next);
    setNavigation((items) => normalizeWorkspaceNavigation(next, items));
  }

  function selectAllFeatures() {
    const next = selected.length === WORKSPACE_FEATURES.length ? [] : WORKSPACE_FEATURES.map((feature) => feature.id);
    setSelected(next);
    setNavigation((items) => normalizeWorkspaceNavigation(next, items));
  }

  function toggleNavigation(item: BottomNavigationItem) {
    setNavigation((items) => items.includes(item)
      ? items.filter((current) => current !== item)
      : [...items.filter((current) => current !== "profile"), item, "profile"]);
  }

  function moveNavigation(item: BottomNavigationItem, direction: -1 | 1) {
    setNavigation((items) => {
      const index = items.indexOf(item);
      const nextIndex = index + direction;
      if (index < 0 || nextIndex < 0 || nextIndex >= items.length) return items;
      const next = [...items];
      [next[index], next[nextIndex]] = [next[nextIndex], next[index]];
      return next;
    });
  }

  async function save() {
    setBusy(true);
    setError("");
    try { await onSave(selected, normalizeWorkspaceNavigation(selected, navigation)); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Не удалось сохранить выбор. Попробуйте ещё раз."); }
    finally { setBusy(false); }
  }
  return <IntroductionDialog titleID="workspace-intro-title" onClose={editing && !busy ? onClose : undefined}>
    <header className="introductionHeading">
      <span className="introductionKicker">identity workspace · ваше пространство</span>
      <h1 id="workspace-intro-title">Соберите{`\n`}пространство под себя</h1>
      <p>Выберите возможности, которые помогут именно вам. Всё лишнее можно скрыть, а выбор — изменить в любой момент.</p>
    </header>
    <form noValidate onSubmit={(event) => { event.preventDefault(); void save(); }}>
      <fieldset className="featureChoices" disabled={busy}>
        <legend className="srOnly">Функции приложения</legend>
        {WORKSPACE_FEATURES.map((feature) => <label className={`featureChoice${selected.includes(feature.id) ? " isSelected" : ""}`} key={feature.id}>
          <input type="checkbox" checked={selected.includes(feature.id)} onChange={(event) => selectFeature(feature.id, event.target.checked)} />
          <span className="featureChoiceIcon"><FeatureIcon feature={feature.id} /></span>
          <span className="featureChoiceCopy"><strong>{feature.title}</strong><span>{feature.description}</span><small>{feature.detail}</small></span>
          <span className="featureChoiceCheck" aria-hidden="true">{selected.includes(feature.id) ? "✓" : "+"}</span>
        </label>)}
      </fieldset>
      <div className="introductionSelection"><span aria-live="polite">Выбрано: {selected.length} из {WORKSPACE_FEATURES.length}</span><button type="button" className="textButton" disabled={busy} onClick={selectAllFeatures}>{selected.length === WORKSPACE_FEATURES.length ? "Снять выбор" : "Выбрать всё"}</button></div>
      <section className="navigationSetup" aria-labelledby="navigation-setup-title">
        <header className="navigationSetupHeading">
          <span>Нижняя панель</span>
          <h2 id="navigation-setup-title">Что покажем снизу?</h2>
          <p>Выберите кнопки и расположите их в удобном порядке.</p>
        </header>
        <NavigationPicker features={selected} navigation={navigation} onToggle={toggleNavigation} onMove={moveNavigation} />
        <div className="navigationAccessHints"><p><strong>Калории</strong> остаются доступны через трекеры на карте профиля и через раздел «Трекер».</p><p><strong>Проекты</strong> остаются доступны через «Портфолио» на карте профиля.</p></div>
      </section>
      <footer className="introductionFooter">
        <p>Выбор можно изменить в профиле. Скрытые функции сохранят ваши данные.</p>
        {error && <p className="introductionError" role="alert">{error}</p>}
        <div className="introductionActions">
          <button className="primaryButton introductionSubmit" disabled={busy || selected.length === 0}>{busy ? "Сохраняем…" : editing ? "Сохранить" : "Открыть моё пространство"}<span aria-hidden="true">→</span></button>
        </div>
        {selected.length === 0 && <small>Выберите хотя бы один инструмент</small>}
      </footer>
    </form>
  </IntroductionDialog>;
}

function NavigationPicker({ features, navigation, onToggle, onMove }: {
  features: WorkspaceFeature[];
  navigation: BottomNavigationItem[];
  onToggle: (item: BottomNavigationItem) => void;
  onMove: (item: BottomNavigationItem, direction: -1 | 1) => void;
}) {
  const candidates = workspaceNavigationCandidates(features);
  const available = [...navigation, ...candidates.filter((item) => !navigation.includes(item))];
  const required = requiredWorkspaceNavigation(features);
  return <div className="navigationPicker">
    <div className="navigationPreview" aria-label="Предпросмотр нижней панели">
      {navigation.map((item) => <span key={item}><NavigationIcon item={item} /><small>{NAVIGATION_LABELS[item]}</small></span>)}
    </div>
    <div className="navigationChoiceList">
      {available.map((item) => {
        const selected = navigation.includes(item);
        const fixed = required.includes(item);
        const index = navigation.indexOf(item);
        return <article className={`navigationChoice${selected ? " isSelected" : ""}`} key={item}>
          <span className="navigationChoiceIcon"><NavigationIcon item={item} /></span>
          <span className="navigationChoiceCopy"><strong>{NAVIGATION_LABELS[item]}</strong><small>{item === "card" || item === "profile" ? "Основной раздел приложения" : fixed ? "Нужна для доступа к выбранной функции" : selected ? "Показывается в нижней панели" : "Доступна с главной страницы"}</small></span>
          {selected && <span className="navigationOrderControls">
            <button type="button" aria-label={`Переместить «${NAVIGATION_LABELS[item]}» влево`} disabled={index === 0} onClick={() => onMove(item, -1)}>←</button>
            <button type="button" aria-label={`Переместить «${NAVIGATION_LABELS[item]}» вправо`} disabled={index === navigation.length - 1} onClick={() => onMove(item, 1)}>→</button>
          </span>}
          <button type="button" className="navigationChoiceToggle" aria-pressed={selected} disabled={fixed} onClick={() => onToggle(item)}>{fixed ? "Всегда" : selected ? "Убрать" : "Добавить"}</button>
        </article>;
      })}
    </div>
  </div>;
}
