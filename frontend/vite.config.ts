import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

// Vite config. The dev server proxies API + WebSocket traffic to the backend
// so the browser only ever talks to one origin.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  server: {
    host: true,
    port: 5173,
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true, ws: true },
      "/health": "http://localhost:8080",
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    // Vitest runs unit tests under src/. The e2e/ directory holds Playwright
    // specs (run via `make e2e`), which must not be collected here.
    include: ["src/**/*.{test,spec}.{ts,tsx}"],
    exclude: ["e2e/**", "node_modules/**", "dist/**"],
  },
});
