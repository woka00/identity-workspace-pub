import type { Content, TableCell, TDocumentDefinitions } from "pdfmake/interfaces";
import type { FatSecretMealNutrition, FatSecretNutrition } from "../../domain/models";

const mealOrder = ["Breakfast", "Lunch", "Dinner", "Other"];
const mealLabels: Record<string, string> = {
  Breakfast: "Завтрак",
  Lunch: "Обед",
  Dinner: "Ужин",
  Other: "Перекус",
};

function number(value: number, digits = 1) {
  return value.toLocaleString("ru-RU", { minimumFractionDigits: 0, maximumFractionDigits: digits });
}

function dateLabel(value: string) {
  const [year, month, day] = value.split("-").map(Number);
  const parsed = new Date(year, month - 1, day, 12);
  return Number.isNaN(parsed.getTime())
    ? value
    : parsed.toLocaleDateString("ru-RU", { weekday: "long", day: "numeric", month: "long", year: "numeric" });
}

function macroRow(label: string, calories: number, protein: number, fat: number, carbohydrate: number, entries?: number): TableCell[] {
  return [
    entries === undefined ? label : `${label}\n${entries} ${entries === 1 ? "запись" : "записей"}`,
    number(calories, 0),
    number(protein),
    number(fat),
    number(carbohydrate),
  ];
}

function mealRows(day: FatSecretNutrition): TableCell[][] {
  const byMeal = new Map(day.meals.map((meal) => [meal.meal, meal]));
  const known = mealOrder.map((meal) => byMeal.get(meal) ?? {
    meal,
    calories: 0,
    protein: 0,
    fat: 0,
    carbohydrate: 0,
    entryCount: 0,
    entries: [],
  });
  const additional = day.meals.filter((meal) => !mealOrder.includes(meal.meal));
  return [...known, ...additional].map((meal: FatSecretMealNutrition) => macroRow(
    mealLabels[meal.meal] ?? meal.meal,
    meal.calories,
    meal.protein,
    meal.fat,
    meal.carbohydrate,
    meal.entryCount,
  ));
}

function sum(days: FatSecretNutrition[], key: "calories" | "protein" | "fat" | "carbohydrate") {
  return days.reduce((total, day) => total + day[key], 0);
}

export async function downloadCalorieReport(days: FatSecretNutrition[], from: string, to: string) {
  const ordered = [...days].sort((left, right) => left.date.localeCompare(right.date));
  const dayCount = Math.max(1, ordered.length);
  const totalCalories = sum(ordered, "calories");
  const totalProtein = sum(ordered, "protein");
  const totalFat = sum(ordered, "fat");
  const totalCarbohydrate = sum(ordered, "carbohydrate");
  const content: Content[] = [
    { text: "Identity Workspace", style: "brand" },
    { text: "Детальный отчёт по калориям и КБЖУ", style: "title" },
    { text: `${dateLabel(from)} — ${dateLabel(to)}`, style: "period" },
    {
      table: {
        headerRows: 1,
        widths: ["*", 62, 54, 54, 62],
        body: [
          ["Период", "ккал", "Белки, г", "Жиры, г", "Углев., г"],
          macroRow("Всего", totalCalories, totalProtein, totalFat, totalCarbohydrate),
          macroRow("В среднем за день", totalCalories / dayCount, totalProtein / dayCount, totalFat / dayCount, totalCarbohydrate / dayCount),
        ],
      },
      layout: "lightHorizontalLines",
      margin: [0, 8, 0, 18],
    },
  ];

  ordered.forEach((day) => {
    content.push(
      { text: dateLabel(day.date), style: "dayTitle", pageBreak: "before" },
      { text: `Итого за день · ${day.entryCount} ${day.entryCount === 1 ? "запись" : "записей"}`, style: "sectionLabel" },
      {
        table: {
          headerRows: 1,
          widths: ["*", 62, 54, 54, 62],
          body: [
            ["", "ккал", "Белки, г", "Жиры, г", "Углев., г"],
            macroRow("Итого", day.calories, day.protein, day.fat, day.carbohydrate),
          ],
        },
        layout: "lightHorizontalLines",
        margin: [0, 5, 0, 16],
      },
      { text: "По приёмам пищи", style: "sectionLabel" },
      {
        table: {
          headerRows: 1,
          widths: ["*", 62, 54, 54, 62],
          body: [
            ["Приём пищи", "ккал", "Белки, г", "Жиры, г", "Углев., г"],
            ...mealRows(day),
          ],
        },
        layout: "lightHorizontalLines",
        margin: [0, 5, 0, 0],
      },
    );
  });

  const definition: TDocumentDefinitions = {
    pageSize: "A4",
    pageMargins: [36, 36, 36, 42],
    info: {
      title: `Отчёт КБЖУ ${from} — ${to}`,
      subject: "Детальная статистика калорий и КБЖУ",
      creator: "Identity Workspace",
    },
    content,
    defaultStyle: { font: "Roboto", fontSize: 9, color: "#191919" },
    styles: {
      brand: { fontSize: 9, bold: true, color: "#777777", margin: [0, 0, 0, 8] },
      title: { fontSize: 21, bold: true, margin: [0, 0, 0, 5] },
      period: { fontSize: 10, color: "#666666", margin: [0, 0, 0, 10] },
      dayTitle: { fontSize: 16, bold: true, margin: [0, 0, 0, 10] },
      sectionLabel: { fontSize: 10, bold: true, color: "#555555", margin: [0, 0, 0, 2] },
    },
    footer: (currentPage, pageCount) => ({
      text: `${currentPage} / ${pageCount}`,
      alignment: "center",
      color: "#888888",
      fontSize: 8,
    }),
  };

  const [pdfMake, pdfFonts] = await Promise.all([
    import("pdfmake/build/pdfmake"),
    import("pdfmake/build/vfs_fonts"),
  ]);
  pdfMake.createPdf(definition, undefined, undefined, pdfFonts.default).download(`identity-workspace-kbzhu-${from}-${to}.pdf`);
}
