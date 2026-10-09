import type { NutritionGoals } from "../domain/models";

export type NutritionCalculatorSex = "female" | "male";
export type NutritionCalculatorActivity = "minimal" | "light" | "moderate" | "high";
export type NutritionCalculatorGoal = "lose" | "maintain" | "gain";
export type NutritionCalculatorPace = "gentle" | "standard";

export interface NutritionCalculatorInput {
  sex: NutritionCalculatorSex;
  age: string;
  heightCm: string;
  weightKg: string;
  activity: NutritionCalculatorActivity;
  goal: NutritionCalculatorGoal;
  pace: NutritionCalculatorPace;
}

export const DEFAULT_NUTRITION_GOALS: NutritionGoals = {
  calorieGoal: 2000,
  proteinGoal: 120,
  fatGoal: 90,
  carbohydrateGoal: 300,
};

export const DEFAULT_NUTRITION_CALCULATOR: NutritionCalculatorInput = {
  sex: "male",
  age: "30",
  heightCm: "175",
  weightKg: "70",
  activity: "moderate",
  goal: "maintain",
  pace: "gentle",
};

export function caloriesFromMacros(protein: number, fat: number, carbohydrate: number) {
  return protein * 4 + fat * 9 + carbohydrate * 4;
}

export function calculateNutritionGoals(input: NutritionCalculatorInput): NutritionGoals | null {
  const age = Number(input.age);
  const heightCm = Number(input.heightCm);
  const weightKg = Number(input.weightKg.replace(",", "."));
  if (!Number.isFinite(age) || age < 18 || age > 100
    || !Number.isFinite(heightCm) || heightCm < 120 || heightCm > 230
    || !Number.isFinite(weightKg) || weightKg < 35 || weightKg > 300) return null;

  const activityFactor: Record<NutritionCalculatorActivity, number> = {
    minimal: 1.2,
    light: 1.375,
    moderate: 1.55,
    high: 1.725,
  };
  const restingEnergy = 10 * weightKg + 6.25 * heightCm - 5 * age + (input.sex === "male" ? 5 : -161);
  const paceAdjustment = input.pace === "standard" ? .15 : .1;
  // A conservative surplus supports muscle gain while limiting unnecessary fat gain.
  // https://pmc.ncbi.nlm.nih.gov/articles/PMC10620361/
  const gainAdjustment = input.pace === "standard" ? .1 : .05;
  const goalFactor = input.goal === "lose" ? 1 - paceAdjustment : input.goal === "gain" ? 1 + gainAdjustment : 1;
  const minimumCalories = input.sex === "female" ? 1200 : 1500;
  const targetCalories = Math.max(minimumCalories, Math.round(restingEnergy * activityFactor[input.activity] * goalFactor));
  const proteinPerKg = input.goal === "gain" ? 2 : input.goal === "maintain" ? 1.6 : 1.8;
  const proteinGoal = Math.max(1, Math.round(weightKg * proteinPerKg));
  const fatGoal = Math.max(1, Math.round(targetCalories * .27 / 9));
  const carbohydrateGoal = Math.max(1, Math.round((targetCalories - proteinGoal * 4 - fatGoal * 9) / 4));
  return {
    proteinGoal,
    fatGoal,
    carbohydrateGoal,
    calorieGoal: caloriesFromMacros(proteinGoal, fatGoal, carbohydrateGoal),
  };
}
