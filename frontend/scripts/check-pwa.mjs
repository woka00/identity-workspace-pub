import { existsSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const publicRoot = join(frontendRoot, "public");
const manifestPath = join(publicRoot, "manifest.webmanifest");
const indexPath = join(frontendRoot, "index.html");
const mainPath = join(frontendRoot, "src", "main.tsx");

const failures = [];
for (const path of [manifestPath, indexPath, mainPath, join(publicRoot, "sw.js")]) {
  if (!existsSync(path)) failures.push(`Missing PWA entry point: ${path}`);
}

if (failures.length === 0) {
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  if (manifest.start_url !== "/") failures.push("manifest start_url must remain /");
  if (manifest.scope !== "/") failures.push("manifest scope must remain /");
  if (manifest.display !== "standalone") failures.push("manifest display must remain standalone");
  for (const icon of manifest.icons ?? []) {
    const iconPath = join(publicRoot, String(icon.src).replace(/^\//, ""));
    if (!existsSync(iconPath)) failures.push(`Missing manifest icon: ${icon.src}`);
  }

  const index = readFileSync(indexPath, "utf8");
  if (!index.includes('rel="manifest"')) failures.push("index.html does not link the web app manifest");
  if (!index.includes('rel="apple-touch-icon"')) failures.push("index.html does not define the iOS home-screen icon");

  const main = readFileSync(mainPath, "utf8");
  if (!main.includes('navigator.serviceWorker.register("/sw.js')) failures.push("main.tsx does not register /sw.js");
}

if (failures.length > 0) {
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log("PWA entry points and assets are valid.");
