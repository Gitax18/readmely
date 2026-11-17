import { describe, expect, it } from "vitest";
import { loadEnv } from "./env";

const ok = { API_INTERNAL_URL: "http://api:8081" };

describe("loadEnv", () => {
  it("accepts a valid URL and defaults to full detail outside production", () => {
    expect(loadEnv(ok)).toEqual({ apiInternalUrl: "http://api:8081", statusDetail: "full" });
  });

  it("defaults to coarse detail in production", () => {
    expect(loadEnv({ ...ok, NODE_ENV: "production" }).statusDetail).toBe("coarse");
  });

  it("lets STATUS_DETAIL override the default in both directions", () => {
    expect(loadEnv({ ...ok, NODE_ENV: "production", STATUS_DETAIL: "full" }).statusDetail).toBe(
      "full",
    );
    expect(loadEnv({ ...ok, STATUS_DETAIL: "coarse" }).statusDetail).toBe("coarse");
  });

  it("strips trailing slashes", () => {
    expect(loadEnv({ API_INTERNAL_URL: "http://api:8081//" }).apiInternalUrl).toBe(
      "http://api:8081",
    );
  });

  it.each([
    ["missing", {}, "API_INTERNAL_URL: is required"],
    ["blank", { API_INTERNAL_URL: "   " }, "API_INTERNAL_URL: is required"],
    ["not a URL", { API_INTERNAL_URL: "nonsense" }, "API_INTERNAL_URL: must be a valid URL"],
    ["wrong scheme", { API_INTERNAL_URL: "ftp://api" }, "scheme must be http or https"],
    ["credentials", { API_INTERNAL_URL: "http://u:p@api" }, "must not contain credentials"],
    ["query", { API_INTERNAL_URL: "http://api/?x=1" }, "must not contain credentials, a query"],
    ["bad detail", { ...ok, STATUS_DETAIL: "verbose" }, "STATUS_DETAIL: must be full or coarse"],
  ])("rejects %s", (_name, env, message) => {
    expect(() => loadEnv(env)).toThrow(message);
  });

  it("reports every problem at once", () => {
    expect(() => loadEnv({ STATUS_DETAIL: "x" })).toThrow(/API_INTERNAL_URL[\s\S]*STATUS_DETAIL/);
  });

  it("never prints a value", () => {
    const secret = "http://admin:hunter2-very-secret@api";
    try {
      loadEnv({ API_INTERNAL_URL: secret, STATUS_DETAIL: "hunter2-detail" });
      expect.unreachable();
    } catch (e) {
      const message = String(e);
      expect(message).not.toContain("hunter2");
      expect(message).not.toContain("admin");
    }
  });
});
