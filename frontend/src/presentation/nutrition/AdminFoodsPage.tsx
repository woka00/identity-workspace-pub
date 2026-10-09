import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "../../infrastructure/http/apiClient";
import type { FoodCatalogReviewInput, UserFoodReview } from "../../domain/models";

type ReviewNumberKey = "caloriesPer100g" | "carbohydratePer100g" | "proteinPer100g" | "fatPer100g";
type ReviewDraft = Omit<FoodCatalogReviewInput, ReviewNumberKey> & Record<ReviewNumberKey, string>;

function number(value: number, digits = 1) {
  return new Intl.NumberFormat("ru-RU", { maximumFractionDigits: digits }).format(value);
}

function date(value: string) {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? "—" : new Intl.DateTimeFormat("ru-RU", { dateStyle: "short", timeStyle: "short" }).format(parsed);
}

function reviewInput(food: UserFoodReview): ReviewDraft {
  return {
    barcode: food.barcode ?? "", name: food.name, brandName: food.brandName ?? "", description: food.description ?? "",
    caloriesPer100g: String(food.caloriesPer100g), carbohydratePer100g: String(food.carbohydratePer100g),
    proteinPer100g: String(food.proteinPer100g), fatPer100g: String(food.fatPer100g),
  };
}

export default function AdminFoodsPage() {
  const [section, setSection] = useState<"foods" | "logs">("foods");
  const [foods, setFoods] = useState<UserFoodReview[]>([]);
  const [pendingOnly, setPendingOnly] = useState(true);
  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<number | null>(null);
  const [busyAction, setBusyAction] = useState<"save" | "promote" | "reject" | null>(null);
  const [editing, setEditing] = useState<UserFoodReview | null>(null);
  const [draft, setDraft] = useState<ReviewDraft | null>(null);
  const [editError, setEditError] = useState("");
  const [error, setError] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsError, setLogsError] = useState("");

  const load = useCallback(async () => {
    setLoading(true); setError("");
    try {
      const result = await api.adminUserFoods(page, pendingOnly);
      setFoods(result.foods); setHasMore(result.hasMore);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось загрузить пользовательские продукты");
    } finally { setLoading(false); }
  }, [page, pendingOnly]);

  const loadLogs = useCallback(async () => {
    setLogsLoading(true); setLogsError("");
    try {
      const result = await api.adminLogs(1000);
      setLogs(result.lines);
    } catch (cause) {
      setLogsError(cause instanceof Error ? cause.message : "Не удалось загрузить логи приложения");
    } finally { setLogsLoading(false); }
  }, []);

  useEffect(() => { if (section === "foods") void load(); }, [load, section]);
  useEffect(() => { if (section === "logs") void loadLogs(); }, [loadLogs, section]);

  function changeFilter(next: boolean) { setPendingOnly(next); setPage(0); }
  function openEditor(food: UserFoodReview) { setEditing(food); setDraft(reviewInput(food)); setEditError(""); }
  function closeEditor() { if (busy !== null) return; setEditing(null); setDraft(null); setEditError(""); }
  function updateDraft<K extends keyof ReviewDraft>(key: K, value: ReviewDraft[K]) {
    setDraft((current) => current ? { ...current, [key]: value } : current);
  }

  async function saveEdit(event: FormEvent) {
    event.preventDefault();
    if (!editing || !draft || busy !== null) return;
    if (!draft.name.trim()) { setEditError("Укажите название продукта"); return; }
    const input: FoodCatalogReviewInput = {
      ...draft,
      caloriesPer100g: Number(draft.caloriesPer100g), proteinPer100g: Number(draft.proteinPer100g),
      fatPer100g: Number(draft.fatPer100g), carbohydratePer100g: Number(draft.carbohydratePer100g),
    };
    const macros = [input.caloriesPer100g, input.proteinPer100g, input.fatPer100g, input.carbohydratePer100g];
    if (macros.some((value) => !Number.isFinite(value) || value < 0) || macros.some((value, index) => value > (index === 0 ? 1000 : 100))) {
      setEditError("Калории должны быть от 0 до 1000, БЖУ — от 0 до 100"); return;
    }
    setBusy(editing.reviewId); setBusyAction("save"); setEditError("");
    try {
      const saved = await api.updateUserFood(editing.reviewId, input);
      setFoods((items) => items.map((item) => item.reviewId === saved.reviewId ? saved : item));
      setEditing(null); setDraft(null);
    } catch (cause) {
      setEditError(cause instanceof Error ? cause.message : "Не удалось сохранить изменения");
    } finally { setBusy(null); setBusyAction(null); }
  }

  async function promote(food: UserFoodReview) {
    if (busy !== null || food.promoted) return;
    setBusy(food.reviewId); setBusyAction("promote"); setError("");
    try {
      await api.promoteUserFood(food.reviewId);
      if (pendingOnly) setFoods((items) => items.filter((item) => item.reviewId !== food.reviewId));
      else setFoods((items) => items.map((item) => item.reviewId === food.reviewId ? { ...item, promoted: true, promotedAt: new Date().toISOString() } : item));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось добавить продукт в глобальную базу");
    } finally { setBusy(null); setBusyAction(null); }
  }

  async function reject(food: UserFoodReview) {
    if (busy !== null || food.promoted || !window.confirm(`Удалить «${food.name}» из очереди на добавление в общую базу? Личный товар пользователя останется доступен только ему.`)) return;
    setBusy(food.reviewId); setBusyAction("reject"); setError("");
    try {
      await api.rejectUserFood(food.reviewId);
      setFoods((items) => items.filter((item) => item.reviewId !== food.reviewId));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Не удалось удалить продукт из очереди");
    } finally { setBusy(null); setBusyAction(null); }
  }

  return (
    <section className="adminFoodsPage" aria-labelledby="admin-foods-title">
      <header className="adminFoodsHero">
        <div><a href="/?view=profile">← Профиль</a><span>Администрирование</span><h1 id="admin-foods-title">{section === "foods" ? "Пользовательские продукты" : "Логи приложения"}</h1><p>{section === "foods" ? "Проверьте и при необходимости исправьте данные перед публикацией для всех пользователей." : "Последние записи текущего процесса приложения. Секретные параметры автоматически скрываются."}</p></div>
        <div className="adminFoodsControls">
          <div className="adminPanelTabs" role="group" aria-label="Раздел администрирования">
            <button type="button" className={section === "foods" ? "isActive" : ""} onClick={() => setSection("foods")}>Товары</button>
            <button type="button" className={section === "logs" ? "isActive" : ""} onClick={() => setSection("logs")}>Логи</button>
          </div>
          {section === "foods" && <div className="adminFoodsFilter" role="group" aria-label="Статус продуктов">
            <button type="button" className={pendingOnly ? "isActive" : ""} onClick={() => changeFilter(true)}>Ожидают</button>
            <button type="button" className={!pendingOnly ? "isActive" : ""} onClick={() => changeFilter(false)}>Все</button>
          </div>}
        </div>
      </header>

      {section === "foods" && <>
        {error && <div className="adminFoodsError" role="alert">{error}</div>}
        <div className="adminFoodsTableViewport">
        <div className="adminFoodsTable" role="table" aria-label="Пользовательские продукты">
          <div className="adminFoodsTableHead" role="row">
            <span>Пользователь</span><span>Штрихкод</span><span>Название и марка</span><span>Ккал</span><span>Белки</span><span>Жиры</span><span>Углеводы</span><span>Добавлен</span><span>Статус</span><span>Действия</span>
          </div>
          {!loading && foods.map((food) => (
            <div className="adminFoodsRow" role="row" key={food.reviewId}>
              <span title={food.ownerLogin}>{food.ownerLogin}</span>
              <code title={food.barcode || "Без штрихкода"}>{food.barcode || "—"}</code>
              <span className="adminFoodName" title={`${food.name}${food.brandName ? ` · ${food.brandName}` : ""}`}><strong>{food.name}</strong><small>{food.brandName || "Без марки"}</small></span>
              <b>{number(food.caloriesPer100g, 0)}</b><span>{number(food.proteinPer100g)}</span><span>{number(food.fatPer100g)}</span><span>{number(food.carbohydratePer100g)}</span>
              <time dateTime={food.submittedAt}>{date(food.submittedAt)}</time>
              <span className={food.promoted ? "adminFoodPublished" : "adminFoodPending"}>{food.promoted ? "В глобальной базе" : "Ожидает"}</span>
              <span className="adminFoodActions">
                <button type="button" className="adminFoodEditButton" disabled={busy !== null} onClick={() => openEditor(food)}>Изменить</button>
                {!food.promoted && <button type="button" className="adminFoodDeleteButton" disabled={busy !== null} onClick={() => void reject(food)}>{busy === food.reviewId && busyAction === "reject" ? "Удаляем…" : "Удалить"}</button>}
                {!food.promoted && <button type="button" disabled={busy !== null} onClick={() => void promote(food)}>{busy === food.reviewId && busyAction === "promote" ? "Добавляем…" : "Добавить"}</button>}
              </span>
            </div>
          ))}
          {loading && <div className="adminFoodsState" role="status">Загружаем продукты…</div>}
          {!loading && foods.length === 0 && <div className="adminFoodsState">{pendingOnly ? "Новых продуктов для проверки нет" : "Пользовательских продуктов пока нет"}</div>}
        </div>
        </div>

        <footer className="adminFoodsPagination">
        <button type="button" disabled={page === 0 || loading} onClick={() => setPage((value) => Math.max(0, value - 1))}>Назад</button><span>Страница {page + 1}</span><button type="button" disabled={!hasMore || loading} onClick={() => setPage((value) => value + 1)}>Дальше</button>
        </footer>
      </>}

      {section === "logs" && <section className="adminLogsPanel" aria-label="Логи приложения">
        <header><div><strong>Последние {logs.length} из 1000 строк</strong><span>Прокручивайте журнал вниз внутри окна. Логи очищаются после перезапуска приложения.</span></div><button type="button" disabled={logsLoading} onClick={() => void loadLogs()}>{logsLoading ? "Обновляем…" : "Обновить"}</button></header>
        {logsError && <div className="adminFoodsError" role="alert">{logsError}</div>}
        <pre aria-live="polite">{logsLoading && logs.length === 0 ? "Загружаем логи…" : logs.length > 0 ? logs.join("\n") : "Записей пока нет"}</pre>
      </section>}

      {editing && draft && (
        <div className="adminFoodEditOverlay" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) closeEditor(); }}>
          <form className="adminFoodEditDialog" noValidate aria-labelledby="admin-food-edit-title" onSubmit={(event) => void saveEdit(event)}>
            <header><div><span>Карточка продукта</span><h2 id="admin-food-edit-title">Изменить данные</h2><p>Личная карточка пользователя останется без изменений.</p></div><button type="button" aria-label="Закрыть" onClick={closeEditor}>×</button></header>
            <div className="adminFoodEditFields">
              <label className="adminFoodEditWide"><span>Название</span><input value={draft.name} maxLength={300} required autoFocus onChange={(event) => updateDraft("name", event.target.value)} /></label>
              <label><span>Марка</span><input value={draft.brandName} maxLength={300} onChange={(event) => updateDraft("brandName", event.target.value)} /></label>
              <label><span>Штрихкод</span><input value={draft.barcode} maxLength={512} onChange={(event) => updateDraft("barcode", event.target.value)} /></label>
              <label><span>Калории, ккал</span><input type="number" min="0" max="1000" step="0.1" required value={draft.caloriesPer100g} onChange={(event) => updateDraft("caloriesPer100g", event.target.value)} /></label>
              <label><span>Белки, г</span><input type="number" min="0" max="100" step="0.1" required value={draft.proteinPer100g} onChange={(event) => updateDraft("proteinPer100g", event.target.value)} /></label>
              <label><span>Жиры, г</span><input type="number" min="0" max="100" step="0.1" required value={draft.fatPer100g} onChange={(event) => updateDraft("fatPer100g", event.target.value)} /></label>
              <label><span>Углеводы, г</span><input type="number" min="0" max="100" step="0.1" required value={draft.carbohydratePer100g} onChange={(event) => updateDraft("carbohydratePer100g", event.target.value)} /></label>
              <label className="adminFoodEditWide"><span>Описание</span><textarea className="resize-none" value={draft.description} maxLength={300} rows={3} onChange={(event) => updateDraft("description", event.target.value)} /></label>
            </div>
            {editError && <div className="adminFoodEditError" role="alert">{editError}</div>}
            <footer><button type="button" onClick={closeEditor} disabled={busy !== null}>Отмена</button><button type="submit" disabled={busy !== null}>{busy === editing.reviewId && busyAction === "save" ? "Сохраняем…" : "Сохранить"}</button></footer>
          </form>
        </div>
      )}
    </section>
  );
}
