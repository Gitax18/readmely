import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import examples from "@/contract/core-internal.examples.json";
import { StatusPanel } from "@/components/status-panel";
import { reportSchema } from "./status-schema";
import { toView } from "./status-view";

// Every response the contract documents for /api/v1/status. The same examples are
// produced by the real Go handler in its conformance tests, so these are real shapes.
const statusExamples = examples["GET /api/v1/status 200"];

describe("contract examples", () => {
  it("documents the six states the page has to cope with", () => {
    expect(Object.keys(statusExamples).sort()).toEqual([
      "ai_down",
      "ai_unconfigured",
      "ai_unreachable",
      "operational",
      "redis_down",
      "unavailable",
    ]);
  });

  it.each(Object.entries(statusExamples))(
    "the Zod schema accepts the %s example",
    (_name, value) => {
      expect(reportSchema.safeParse(value).success).toBe(true);
    },
  );

  it.each(Object.entries(statusExamples))(
    "the page renders the %s example in both detail levels",
    (_name, value) => {
      const parsed = reportSchema.parse(value);
      for (const detail of ["full", "coarse"] as const) {
        const { container, unmount } = render(
          <StatusPanel view={toView({ kind: "report", report: parsed }, detail)} />,
        );
        // The overall heading names the state; it must never come out empty.
        expect(container.querySelector("#overall")?.textContent?.trim()).toBeTruthy();
        unmount();
      }
    },
  );

  it("exercises every value the schema allows, so no enum member goes untested", () => {
    const overall = new Set<string>();
    const service = new Set<string>();
    for (const ex of Object.values(statusExamples)) {
      overall.add(ex.status);
      service.add(ex.services.api.status);
      service.add(ex.services.ai.status);
    }
    expect([...overall].sort()).toEqual(["degraded", "operational", "unavailable"]);
    expect([...service].sort()).toEqual(["down", "ok", "unconfigured", "unreachable"]);
  });

  it("the schema rejects an example the contract would also reject", () => {
    const broken = structuredClone(statusExamples.operational) as { status: string };
    broken.status = "fine";
    expect(reportSchema.safeParse(broken).success).toBe(false);
  });
});
