import type { Overall, ServiceStatus } from "./status-schema";
import type { StatusResult } from "./status-client";
import type { StatusDetail } from "./env";

export interface CheckView {
  name: string;
  state: "ok" | "down";
}

export interface ServiceView {
  status: ServiceStatus;
  /** null when the detail level hides dependencies. */
  checks: CheckView[] | null;
}

export type StatusView =
  | { kind: "report"; overall: Overall; api: ServiceView; ai: ServiceView }
  | { kind: "api_unreachable" }
  | { kind: "invalid" };

/**
 * Builds what the browser is allowed to see. Redaction happens here, on the
 * server, so hidden detail is never part of the rendered page.
 */
export function toView(result: StatusResult, detail: StatusDetail): StatusView {
  if (result.kind !== "report") return result;
  const { report } = result;
  const view = (s: (typeof report.services)["api"]): ServiceView => ({
    status: s.status,
    checks:
      detail === "full"
        ? Object.entries(s.checks)
            .sort(([a], [b]) => a.localeCompare(b))
            .map(([name, state]) => ({ name, state }))
        : null,
  });
  return {
    kind: "report",
    overall: report.status,
    api: view(report.services.api),
    ai: view(report.services.ai),
  };
}
