import type { Report } from "@/lib/status-schema";

export function report(overrides: Partial<Report> = {}): Report {
  return {
    status: "operational",
    services: {
      api: { status: "ok", checks: { postgres: "ok", redis: "ok" } },
      ai: { status: "ok", checks: { postgres: "ok" } },
    },
    ...overrides,
  };
}
