import { defineConfig, devices } from "@playwright/test";

const FAKE_API = "http://127.0.0.1:4010";
const env = (detail: string) => ({
  API_INTERNAL_URL: FAKE_API,
  STATUS_DETAIL: detail,
  NODE_ENV: "production",
  NEXT_TELEMETRY_DISABLED: "1",
});

export default defineConfig({
  testDir: "./e2e",
  // Tests switch the fake API's scenario, so they must not run in parallel.
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: { baseURL: "http://127.0.0.1:3100", ...devices["Desktop Chrome"] },
  webServer: [
    {
      command: "node e2e/fake-api.mjs",
      url: `${FAKE_API}/__health`,
      reuseExistingServer: false,
    },
    {
      // The real production build, with full status detail.
      command: "pnpm run build && pnpm exec next start --hostname 127.0.0.1 --port 3100",
      url: "http://127.0.0.1:3100/healthz",
      timeout: 240_000,
      env: env("full"),
      reuseExistingServer: false,
    },
    {
      // Same build, coarse detail (production's default view).
      command: "pnpm exec next start --hostname 127.0.0.1 --port 3101",
      url: "http://127.0.0.1:3101/healthz",
      timeout: 240_000,
      env: env("coarse"),
      reuseExistingServer: false,
    },
  ],
});
