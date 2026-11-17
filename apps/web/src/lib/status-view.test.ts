import { describe, expect, it } from "vitest";
import { report } from "@/test/fixtures";
import { toView } from "./status-view";

describe("toView", () => {
  it("keeps sorted dependency detail at the full level", () => {
    const v = toView({ kind: "report", report: report() }, "full");
    if (v.kind !== "report") throw new Error("expected a report");
    expect(v.api.checks).toEqual([
      { name: "postgres", state: "ok" },
      { name: "redis", state: "ok" },
    ]);
  });

  it("drops all dependency detail at the coarse level", () => {
    const v = toView({ kind: "report", report: report() }, "coarse");
    if (v.kind !== "report") throw new Error("expected a report");
    expect(v.api.checks).toBeNull();
    expect(v.ai.checks).toBeNull();
    expect(JSON.stringify(v)).not.toMatch(/postgres|redis/);
  });

  it("passes failures through unchanged", () => {
    expect(toView({ kind: "api_unreachable" }, "coarse")).toEqual({ kind: "api_unreachable" });
    expect(toView({ kind: "invalid" }, "full")).toEqual({ kind: "invalid" });
  });
});
