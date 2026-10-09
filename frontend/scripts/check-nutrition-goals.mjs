import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import ts from "typescript";

const source = await readFile(new URL("../src/application/nutritionGoals.ts", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } });
const { calculateNutritionGoals, DEFAULT_NUTRITION_CALCULATOR } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`);
const input = { ...DEFAULT_NUTRITION_CALCULATOR, goal: "gain" };
assert.deepEqual(calculateNutritionGoals(input), { calorieGoal: 2684, proteinGoal: 140, fatGoal: 80, carbohydrateGoal: 351 });
assert.deepEqual(calculateNutritionGoals({ ...input, pace: "standard" }), { calorieGoal: 2812, proteinGoal: 140, fatGoal: 84, carbohydrateGoal: 374 });
for (const sex of ["male", "female"]) {
  for (const activity of ["minimal", "light", "moderate", "high"]) {
    const values = ["lose", "maintain", "gain"].map((goal) => calculateNutritionGoals({ ...input, sex, activity, goal }));
    assert.ok(values[0].calorieGoal < values[1].calorieGoal && values[1].calorieGoal < values[2].calorieGoal);
    for (const result of values) {
      assert.equal(result.calorieGoal, result.proteinGoal * 4 + result.fatGoal * 9 + result.carbohydrateGoal * 4);
      assert.ok(result.proteinGoal > 0 && result.fatGoal > 0 && result.carbohydrateGoal > 0);
    }
    assert.ok(values[2].calorieGoal <= values[1].calorieGoal * 1.06, "gentle muscle gain must have a small surplus");
  }
}
assert.deepEqual(calculateNutritionGoals({ ...input, weightKg: "70,5" }), calculateNutritionGoals({ ...input, weightKg: "70.5" }));
for (const invalid of [{ age: "17" }, { weightKg: "" }, { heightCm: "NaN" }]) {
  assert.equal(calculateNutritionGoals({ ...input, ...invalid }), null);
}
console.log("Nutrition calculator regression checks passed.");
