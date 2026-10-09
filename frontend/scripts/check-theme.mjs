import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import ts from "typescript";

const source = await readFile(new URL("../src/application/theme.ts", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } });
const { DEFAULT_THEME, normalizeTheme, validThemeTime, isDarkTheme, resolveTheme } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`);
const at = (time) => new Date(`2026-09-16T${time}:00`);
const overnight = { mode: "scheduled", darkStart: "22:00", darkEnd: "07:00" };
for (const [time, expected] of [["21:59", false], ["22:00", true], ["23:59", true], ["00:00", true], ["06:59", true], ["07:00", false]]) {
  assert.equal(isDarkTheme(overnight, at(time)), expected, `overnight boundary ${time}`);
}
const daytime = { mode: "scheduled", darkStart: "09:15", darkEnd: "17:45" };
for (const [time, expected] of [["09:14", false], ["09:15", true], ["17:44", true], ["17:45", false], ["00:00", false]]) {
  assert.equal(isDarkTheme(daytime, at(time)), expected, `daytime boundary ${time}`);
}
for (const time of ["00:00", "12:00", "23:59"]) {
  assert.equal(isDarkTheme({ ...overnight, mode: "light" }, at(time)), false);
  assert.equal(isDarkTheme({ ...overnight, mode: "dark" }, at(time)), true);
  assert.equal(resolveTheme({ ...overnight, mode: "apple" }, at(time)), "apple");
}
for (const invalid of [null, undefined, "", "7:00", "24:00", "12:60", "12:00:00", 123]) assert.equal(validThemeTime(invalid), false);
for (const invalid of [null, [], "dark", { mode: "invalid" }, { darkStart: "25:00", darkEnd: "09:00" }, { darkStart: "09:00", darkEnd: "09:00" }]) assert.deepEqual(normalizeTheme(invalid), DEFAULT_THEME);
assert.deepEqual(normalizeTheme(daytime), daytime);
assert.deepEqual(normalizeTheme({ ...daytime, mode: "apple" }), { ...daytime, mode: "apple" });

// Guard the palette: geometry stays in styles.css; theme tokens only invert RGB and preserve alpha.
const css = await readFile(new URL("../src/styles.css", import.meta.url), "utf8");
const theme = await readFile(new URL("../src/theme.css", import.meta.url), "utf8");
const palette = new Map([...theme.matchAll(/(--mono-[\w-]+):\s*([^;]+);/g)].map(([, key, value]) => [key, value]));
function rgba(value) {
  if (value.startsWith("#")) {
    let hex = value.slice(1);
    if (hex.length <= 4) hex = [...hex].map(c => c + c).join("");
    return [0, 2, 4].map(i => parseInt(hex.slice(i, i + 2), 16)).concat(hex.length === 8 ? parseInt(hex.slice(6), 16) / 255 : 1);
  }
  const parts = value.match(/[\d.]+/g).map(Number);
  return [...parts.slice(0, 3), parts[3] ?? 1];
}
for (const [, name, fallback] of css.matchAll(/var\((--mono-[\w-]+),\s*(#[\da-f]+|rgba?\([^)]*\))\)/gi)) {
  assert.ok(palette.has(name), `missing dark color ${name}`);
  const light = rgba(fallback);
  const dark = rgba(palette.get(name));
  assert.deepEqual(dark, light.map((value, index) => index === 3 ? value : 255 - value), name);
}
for (const token of ["--apple-blue", "--app-bg", "--tile", "--tile-strong", "--tile-soft"]) {
  assert.match(theme, new RegExp(`:root\\[data-theme="apple"\\][\\s\\S]*?${token}:`), `missing Apple theme token ${token}`);
}
assert.match(theme, /:root\[data-theme="apple"\][\s\S]*?prefers-reduced-transparency/, "Apple theme must provide a reduced-transparency fallback");
const appleMobile = theme.slice(theme.lastIndexOf("@media (max-width: 760px)"));
for (const selector of [".portfolioDigest", ".taskDocument", ".mobileCreateForm.taskEditorForm .taskEditorActions"]) {
  assert.ok(appleMobile.includes(selector), `missing Apple mobile override for ${selector}`);
}
console.log("Theme schedule boundaries, preferences, inverse palette and Apple theme checks passed.");
