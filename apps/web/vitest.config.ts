import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

const src = (p: string) => fileURLToPath(new URL(p, import.meta.url));

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": src("./src"),
      // `server-only` throws outside a React server build; tests run in plain Node.
      "server-only": src("./src/test/server-only.ts"),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    coverage: {
      provider: "v8",
      include: ["src/lib/**", "src/components/**"],
      // contract-check.ts and *.fixture.ts hold only compile-time checks (verified by `tsc`).
      exclude: [
        "src/**/*.test.{ts,tsx}",
        "src/test/**",
        "src/lib/contract-check.ts",
        "src/**/*.fixture.ts",
      ],
      // Lowered while version 1 is built without new tests (plans/v1-roadmap.md); Phase 10
      // restores 90/90/85/90 after backfilling tests for the security-critical code. The Phase 9
      // interface rebuild and the private-repository picker added screens and components without
      // tests, so the floor dropped again (a deliberate decision by the product owner, 2026-11-24).
      thresholds: { lines: 29, functions: 21, branches: 29, statements: 29 },
    },
  },
});
