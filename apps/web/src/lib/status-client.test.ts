import { describe, expect, it, vi } from "vitest";
import { report } from "@/test/fixtures";
import { fetchStatus, MAX_BODY_BYTES } from "./status-client";

const json = (body: unknown, init?: ResponseInit) => new Response(JSON.stringify(body), init);
const returns = (res: Response) => vi.fn().mockResolvedValue(res) as unknown as typeof fetch;

describe("fetchStatus", () => {
  it("returns the report and calls the right URL with safe options", async () => {
    const f = vi.fn().mockResolvedValue(json(report()));
    const result = await fetchStatus("http://api:8081", {
      fetchImpl: f as unknown as typeof fetch,
    });
    expect(result).toEqual({ kind: "report", report: report() });
    const [url, init] = f.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://api:8081/api/v1/status");
    expect(init.cache).toBe("no-store");
    expect(init.redirect).toBe("error");
  });

  it("treats a network error as unreachable", async () => {
    const f = vi.fn().mockRejectedValue(new TypeError("fetch failed")) as unknown as typeof fetch;
    expect(await fetchStatus("http://api:8081", { fetchImpl: f })).toEqual({
      kind: "api_unreachable",
    });
  });

  it("times out a hanging API and answers promptly", async () => {
    const hang = ((_url: string, init: RequestInit) =>
      new Promise((_resolve, reject) => {
        init.signal?.addEventListener("abort", () =>
          reject(new DOMException("aborted", "TimeoutError")),
        );
      })) as unknown as typeof fetch;
    const start = Date.now();
    const result = await fetchStatus("http://api:8081", { fetchImpl: hang, timeoutMs: 50 });
    expect(result).toEqual({ kind: "api_unreachable" });
    expect(Date.now() - start).toBeLessThan(1000);
  });

  it.each([404, 500, 502, 503])("treats HTTP %i as unreachable", async (status) => {
    expect(
      await fetchStatus("http://api:8081", { fetchImpl: returns(new Response("x", { status })) }),
    ).toEqual({
      kind: "api_unreachable",
    });
  });

  it("treats a redirect (rejected by redirect: error) as unreachable", async () => {
    const f = vi.fn().mockRejectedValue(new TypeError("redirect")) as unknown as typeof fetch;
    expect(await fetchStatus("http://api:8081", { fetchImpl: f })).toEqual({
      kind: "api_unreachable",
    });
  });

  it.each([
    ["not JSON", new Response("<html>")],
    ["wrong shape", json({ status: "operational" })],
    ["hostile values", json({ ...report(), status: "<img src=x onerror=alert(1)>" })],
  ])("treats %s as invalid", async (_name, res) => {
    expect(await fetchStatus("http://api:8081", { fetchImpl: returns(res) })).toEqual({
      kind: "invalid",
    });
  });

  it("rejects an oversized body without reading all of it", async () => {
    const big = "a".repeat(MAX_BODY_BYTES + 10);
    const result = await fetchStatus("http://api:8081", { fetchImpl: returns(new Response(big)) });
    expect(result).toEqual({ kind: "invalid" });
  });

  it("treats an empty body as invalid", async () => {
    expect(
      await fetchStatus("http://api:8081", { fetchImpl: returns(new Response(null)) }),
    ).toEqual({
      kind: "invalid",
    });
  });
});
