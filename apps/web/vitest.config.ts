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
      thresholds: { lines: 90, functions: 90, branches: 85, statements: 90 },
    },
  },
});
