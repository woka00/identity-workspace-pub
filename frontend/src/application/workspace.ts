import type { BottomNavigationItem, WorkspaceFeature, WorkspacePreferences } from "../domain/models";

export const WORKSPACE_FEATURES: { id: WorkspaceFeature; title: string; description: string; detail: string }[] = [
  { id: "tracker", title: "Таймер", description: "Понимать, на что уходит время", detail: "Секундомер для работы, учёбы и отдыха. История и статистика активностей." },
  { id: "calories", title: "Учёт калорий", description: "Следить за своим питанием", detail: "Дневник еды, калории и БЖУ — внутри приложения или через FatSecret." },
  { id: "tasks", title: "Задачи", description: "Освободить голову от списка дел", detail: "Планы на день и повторяющиеся задачи внутри identity workspace." },
  { id: "projects", title: "Работа", description: "Вести проекты вместе", detail: "Обзор, списки задач, участники, файлы и прогресс проектов." },
  { id: "widgets", title: "Виджеты и трекеры", description: "Держать важное перед глазами", detail: "Карточки воды, веса, GitHub и своих привычек под картой профиля." },
];

export function workspaceNavigationCandidates(features: WorkspaceFeature[]): BottomNavigationItem[] {
  const featureItems = features.filter((feature): feature is Exclude<WorkspaceFeature, "widgets"> => feature !== "widgets");
  return ["card", ...featureItems, "profile"];
}

export function requiredWorkspaceNavigation(features: WorkspaceFeature[]): BottomNavigationItem[] {
  const required: BottomNavigationItem[] = ["card"];
  if (features.includes("tasks")) required.push("tasks");
  if (features.includes("tracker")) required.push("tracker");
  if (features.includes("calories") && !features.includes("tracker") && !features.includes("widgets")) required.push("calories");
  required.push("profile");
  return required;
}

export function normalizeWorkspaceNavigation(features: WorkspaceFeature[], order: BottomNavigationItem[]): BottomNavigationItem[] {
  const available = workspaceNavigationCandidates(features);
  const required = requiredWorkspaceNavigation(features);
  const selected = order.filter((item, index) => available.includes(item) && order.indexOf(item) === index);
  for (const item of required) {
    if (selected.includes(item)) continue;
    if (item === "card") selected.unshift(item);
    else if (item === "profile") selected.push(item);
    else selected.splice(selected.indexOf("profile") < 0 ? selected.length : selected.indexOf("profile"), 0, item);
  }
  return selected;
}

export function workspaceNavigation(preferences: WorkspacePreferences | null, order: BottomNavigationItem[]): BottomNavigationItem[] {
  if (!preferences?.onboardingCompleted) return order;
  return normalizeWorkspaceNavigation(preferences.features, order);
}
