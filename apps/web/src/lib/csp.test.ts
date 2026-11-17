import { describe, expect, it } from "vitest";
import { buildCsp, newNonce } from "./csp";

const directive = (csp: string, name: string) =>
  csp.split("; ").find((d) => d === name || d.startsWith(`${name} `)) ?? "";

describe("buildCsp", () => {
  const prod = buildCsp("abc123", false);

  it("puts the nonce on script and style sources", () => {
    expect(directive(prod, "script-src")).toContain("'nonce-abc123'");
    expect(directive(prod, "style-src")).toContain("'nonce-abc123'");
  });

  it("allows no inline script and no eval in production", () => {
    expect(prod).not.toContain("unsafe-inline");
    expect(prod).not.toContain("unsafe-eval");
  });

  it("adds eval and websockets only in development", () => {
    const dev = buildCsp("abc123", true);
    expect(directive(dev, "script-src")).toContain("'unsafe-eval'");
    expect(directive(dev, "connect-src")).toContain("ws:");
    expect(directive(prod, "connect-src")).toBe("connect-src 'self'");
  });

  it.each([
    "default-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ])("always contains %s", (d) => {
    expect(directive(prod, d.split(" ")[0]!)).toBe(d);
  });

  it("never allows wildcard or remote sources", () => {
    expect(prod).not.toMatch(/\*|https?:\/\//);
  });
});

describe("newNonce", () => {
  it("is unique and long enough on every call", () => {
    const nonces = new Set(Array.from({ length: 500 }, newNonce));
    expect(nonces.size).toBe(500);
    for (const n of nonces) expect(n).toMatch(/^[A-Za-z0-9+/]{22}==$/);
  });
});
