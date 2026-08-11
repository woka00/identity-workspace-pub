import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
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
