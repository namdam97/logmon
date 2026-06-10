import { defineConfig, devices } from "@playwright/test";

// E2E chạy với FE đã build (next start). Backend (userservice + Postgres) phải
// được dựng sẵn ở localhost:8080 — xem doc/e2e hoặc docker compose.
export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: "http://localhost:3000",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  // Dùng Google Chrome hệ thống (Playwright chưa có chromium bundled cho OS này).
  projects: [
    { name: "chrome", use: { ...devices["Desktop Chrome"], channel: "chrome" } },
  ],
  webServer: {
    command: "node_modules/.bin/next start",
    url: "http://localhost:3000",
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
});
