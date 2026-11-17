import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";
import { config, proxy } from "./proxy";

const call = () => proxy(new NextRequest("http://localhost:3000/"));
const nonceOf = (csp: string) => /'nonce-([^']+)'/.exec(csp)?.[1];

describe("proxy", () => {
  it("sets a CSP whose nonce differs on every request", () => {
    const a = call().headers.get("content-security-policy") ?? "";
    const b = call().headers.get("content-security-policy") ?? "";
    expect(nonceOf(a)).toBeTruthy();
    expect(nonceOf(a)).not.toBe(nonceOf(b));
  });

  it("forwards the same nonce and policy to the render through request headers", () => {
    const res = call();
    const csp = res.headers.get("content-security-policy");
    expect(res.headers.get("x-middleware-request-content-security-policy")).toBe(csp);
    expect(res.headers.get("x-middleware-request-x-nonce")).toBe(nonceOf(csp ?? ""));
  });

  it("skips static assets and prefetches", () => {
    const [m] = config.matcher;
    expect(m?.source).toContain("_next/static");
    expect(m?.missing?.length).toBe(2);
  });
});
