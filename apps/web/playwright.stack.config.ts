import { defineConfig, devices } from "@playwright/test";

// Browser tests against the REAL stack (web, API, Postgres, Redis), which must already be
// running: `make up-app`, then `make test-e2e-stack`. The sign-in code is read from the API's
// development mailer, which logs it (the log mailer is refused in production).
export default defineConfig({
  testDir: "./e2e-stack",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  timeout: 60_000,
  use: { baseURL: process.env.STACK_URL ?? "http://127.0.0.1:3000", ...devices["Desktop Chrome"] },
});
