import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const sourceRoot = join(frontendRoot, "src");
const rules = new Map([
  ["domain", ["application", "infrastructure", "presentation"]],
  ["application", ["infrastructure", "presentation"]],
  ["infrastructure", ["application", "presentation"]],
]);

function sourceFiles(directory) {
  return readdirSync(directory).flatMap((name) => {
    const path = join(directory, name);
    if (statSync(path).isDirectory()) return sourceFiles(path);
    return /\.(ts|tsx)$/.test(name) ? [path] : [];
  });
}

const failures = [];
for (const file of sourceFiles(sourceRoot)) {
  const pathFromSource = relative(sourceRoot, file);
  const [layer] = pathFromSource.split(sep);
  const forbiddenLayers = rules.get(layer);
  if (!forbiddenLayers) continue;

  const content = readFileSync(file, "utf8");
  const imports = content.matchAll(/(?:from\s+|import\s*)["']([^"']+)["']/g);
  for (const match of imports) {
    const imported = match[1];
    if (!imported.startsWith(".")) continue;
    const resolvedImport = resolve(dirname(file), imported);
    const importedPath = relative(sourceRoot, resolvedImport);
    const [importedLayer] = importedPath.split(sep);
    if (forbiddenLayers.includes(importedLayer)) {
      failures.push(`${pathFromSource} must not import ${importedPath}`);
    }
  }
}

if (failures.length > 0) {
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log("Frontend architecture boundaries are valid.");
