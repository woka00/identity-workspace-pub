import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const tesseractCoreFiles = [
  "tesseract-core-lstm.wasm.js",
  "tesseract-core-lstm.wasm",
  "tesseract-core-simd-lstm.wasm.js",
  "tesseract-core-simd-lstm.wasm",
];

function localTesseractAssets() {
  const assetPath = (url: string) => {
    if (url === "/tesseract/worker.min.js") return resolve("node_modules/tesseract.js/dist/worker.min.js");
    const prefix = "/tesseract/core/";
    if (!url.startsWith(prefix)) return "";
    const fileName = url.slice(prefix.length);
    return tesseractCoreFiles.includes(fileName) ? resolve("node_modules/tesseract.js-core", fileName) : "";
  };
  return {
    name: "local-tesseract-assets",
    configureServer(server: { middlewares: { use: (handler: (request: { url?: string }, response: { statusCode: number; setHeader: (name: string, value: string) => void; end: (body?: Uint8Array) => void }, next: () => void) => void) => void } }) {
      server.middlewares.use((request, response, next) => {
        const filePath = assetPath(request.url ?? "");
        if (!filePath) return next();
        response.statusCode = 200;
        response.setHeader("Content-Type", filePath.endsWith(".wasm") ? "application/wasm" : "text/javascript; charset=utf-8");
        response.end(readFileSync(filePath));
      });
    },
    generateBundle(this: { emitFile: (file: { type: "asset"; fileName: string; source: Uint8Array }) => void }) {
      this.emitFile({
        type: "asset",
        fileName: "tesseract/worker.min.js",
        source: readFileSync(resolve("node_modules/tesseract.js/dist/worker.min.js")),
      });
      for (const fileName of tesseractCoreFiles) {
        this.emitFile({
          type: "asset",
          fileName: `tesseract/core/${fileName}`,
          source: readFileSync(resolve("node_modules/tesseract.js-core", fileName)),
        });
      }
    },
  };
}

export default defineConfig({
  plugins: [react(), localTesseractAssets()],
  build: {
    sourcemap: false,
  },
  server: {
    // Development server is intentionally loopback-only. Use an explicit
    // --host value only on a trusted LAN when mobile-device testing is needed.
    host: "127.0.0.1",
    strictPort: true,
    cors: false,
    proxy: { "/api": "http://127.0.0.1:8080" },
  },
  preview: {
    host: "127.0.0.1",
    strictPort: true,
  },
});
