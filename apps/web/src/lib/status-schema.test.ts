import { describe, expect, it } from "vitest";
import { report } from "@/test/fixtures";
import { reportSchema } from "./status-schema";

const parse = (v: unknown) => reportSchema.safeParse(v).success;

describe("reportSchema", () => {
  it("accepts a well-formed report", () => {
    expect(parse(report())).toBe(true);
  });

  it("accepts empty checks (unreachable or unconfigured services)", () => {
    const r = report();
    r.services.ai = { status: "unreachable", checks: {} };
    expect(parse(r)).toBe(true);
  });

  it.each([
    ["null", null],
    ["array", [1, 2]],
    ["string", "ok"],
    ["empty object", {}],
    ["unknown overall status", { ...report(), status: "fine" }],
    [
      "unknown service status",
      (() => {
        const r = report();
        (r.services.api as { status: string }).status = "up";
        return r;
      })(),
    ],
    ["unknown top-level key", { ...report(), debug: "x" }],
    [
      "unknown service key",
      (() => {
        const r = report();
        (r.services.ai as Record<string, unknown>).secret = "x";
        return r;
      })(),
    ],
    [
      "extra service",
      { ...report(), services: { ...report().services, db: { status: "ok", checks: {} } } },
    ],
    ["missing ai", { status: "operational", services: { api: report().services.api } }],
    [
      "check value not ok/down",
      (() => {
        const r = report();
        (r.services.api.checks as Record<string, string>).postgres = "maybe";
        return r;
      })(),
    ],
    [
      "check name with markup",
      (() => {
        const r = report();
        r.services.api.checks = { "<script>": "ok" };
        return r;
      })(),
    ],
    [
      "check name with uppercase",
      (() => {
        const r = report();
        r.services.api.checks = { Postgres: "ok" };
        return r;
      })(),
    ],
    [
      "too many checks",
      (() => {
        const r = report();
        r.services.api.checks = Object.fromEntries(
          Array.from({ length: 9 }, (_, i) => [`c${i}`, "ok"]),
        ) as never;
        return r;
      })(),
    ],
    [
      "checks as array",
      (() => {
        const r = report();
        (r.services.api as { checks: unknown }).checks = ["ok"];
        return r;
      })(),
    ],
  ])("rejects %s", (_name, value) => {
    expect(parse(value)).toBe(false);
  });

  it("drops a __proto__ key smuggled in through JSON without polluting anything", () => {
    const json =
      '{"status":"operational","services":{"api":{"status":"ok","checks":{"__proto__":"ok","redis":"down"}},"ai":{"status":"ok","checks":{}}}}';
    const result = reportSchema.safeParse(JSON.parse(json));
    expect(result.success).toBe(true);
    if (!result.success) return;
    const checks = result.data.services.api.checks;
    expect(Object.prototype.hasOwnProperty.call(checks, "__proto__")).toBe(false);
    expect(checks).toEqual({ redis: "down" });
    expect(({} as Record<string, unknown>).ok).toBeUndefined();
  });
});
