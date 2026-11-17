import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { report } from "@/test/fixtures";
import { toView } from "@/lib/status-view";
import { StatusPanel } from "./status-panel";

const view = (r = report(), detail: "full" | "coarse" = "full") =>
  toView({ kind: "report", report: r }, detail);

describe("StatusPanel", () => {
  it("shows operational", () => {
    render(<StatusPanel view={view()} />);
    expect(
      screen.getByRole("heading", { level: 2, name: /all systems operational/i }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Operational").length).toBeGreaterThanOrEqual(2);
  });

  it("shows degraded with the AI service down", () => {
    const r = report({ status: "degraded" });
    r.services.ai = { status: "down", checks: { postgres: "down" } };
    render(<StatusPanel view={view(r)} />);
    expect(
      screen.getByRole("heading", { level: 2, name: /some services are degraded/i }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Down").length).toBeGreaterThan(0);
  });

  it("shows unavailable", () => {
    const r = report({ status: "unavailable" });
    r.services.api = { status: "down", checks: { postgres: "down", redis: "ok" } };
    render(<StatusPanel view={view(r)} />);
    expect(
      screen.getByRole("heading", { level: 2, name: /service unavailable/i }),
    ).toBeInTheDocument();
  });

  it("shows a not-configured AI service", () => {
    const r = report({ status: "degraded" });
    r.services.ai = { status: "unconfigured", checks: {} };
    render(<StatusPanel view={view(r)} />);
    expect(screen.getByText("Not configured")).toBeInTheDocument();
  });

  it("shows when the API cannot be reached", () => {
    render(<StatusPanel view={{ kind: "api_unreachable" }} />);
    expect(
      screen.getByRole("heading", { level: 2, name: /can't reach the readmely api/i }),
    ).toBeInTheDocument();
    expect(screen.queryByText("AI service")).not.toBeInTheDocument();
  });

  it("shows an invalid response", () => {
    render(<StatusPanel view={{ kind: "invalid" }} />);
    expect(
      screen.getByRole("heading", { level: 2, name: /invalid response/i }),
    ).toBeInTheDocument();
  });

  it("lists dependencies at the full level", () => {
    render(<StatusPanel view={view(report(), "full")} />);
    expect(screen.getAllByText("postgres").length).toBe(2);
    expect(screen.getByText("redis")).toBeInTheDocument();
  });

  it("leaves dependency names out of the HTML at the coarse level", () => {
    const { container } = render(<StatusPanel view={view(report(), "coarse")} />);
    expect(container.innerHTML).not.toMatch(/postgres|redis/i);
    expect(screen.getByText("API")).toBeInTheDocument();
  });

  it("conveys status with text, not colour alone", () => {
    render(<StatusPanel view={view()} />);
    for (const badge of screen.getAllByText("Operational")) {
      expect(badge.textContent?.trim()).toBeTruthy();
    }
    expect(document.querySelectorAll("svg[aria-hidden='true']").length).toBeGreaterThan(0);
  });
});
