import { describe, expect, it, vi } from "vitest";
import { identity, json, me, session } from "@/test/account-fixtures";
import { fetchIdentities, fetchProviders, fetchSession, fetchSessions } from "./account-client";

const BASE = "http://api:8080";

describe("fetchSession", () => {
  it("is signed out without a cookie, and does not call the API", async () => {
    const fetchImpl = vi.fn<typeof fetch>();
    expect(await fetchSession(BASE, undefined, { fetchImpl })).toEqual({ kind: "signed_out" });
    expect(await fetchSession(BASE, "", { fetchImpl })).toEqual({ kind: "signed_out" });
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it("returns the user and sends the cookie to /v1/me", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => json(me()));
    const result = await fetchSession(BASE, "tok", { fetchImpl });
    expect(result).toEqual({ kind: "signed_in", me: me() });
    expect(fetchImpl.mock.calls[0]?.[0]).toBe(`${BASE}/v1/me`);
    const init = fetchImpl.mock.calls[0]?.[1];
    expect(new Headers(init?.headers).get("cookie")).toBe("__Host-rm_session=tok");
    expect(init?.cache).toBe("no-store");
    expect(init?.redirect).toBe("error");
  });

  it("is signed out on 401, which is not an error", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () =>
      json({ error: "unauthenticated" }, { status: 401 }),
    );
    expect(await fetchSession(BASE, "tok", { fetchImpl })).toEqual({ kind: "signed_out" });
  });

  it.each([
    ["a server error", async () => json({ error: "internal_error" }, { status: 500 })],
    ["a network failure", async () => Promise.reject(new Error("ECONNREFUSED"))],
    ["a body that is not JSON", async () => new Response("<html>", { status: 200 })],
    ["a body of the wrong shape", async () => json({ user: { id: "x" } })],
    ["a CSRF token of the wrong shape", async () => json({ ...me(), csrf_token: "short" })],
  ])("reports an error, not signed out, for %s", async (_name, impl) => {
    const fetchImpl = vi.fn<typeof fetch>(impl);
    expect(await fetchSession(BASE, "tok", { fetchImpl })).toEqual({ kind: "error" });
  });

  it("gives up on a slow API", async () => {
    const fetchImpl = vi.fn<typeof fetch>(
      (_u, init) =>
        new Promise((_r, reject) =>
          init?.signal?.addEventListener("abort", () => reject(new Error("t"))),
        ),
    );
    expect(await fetchSession(BASE, "tok", { fetchImpl, timeoutMs: 20 })).toEqual({
      kind: "error",
    });
  });
});

describe("fetchSessions", () => {
  it("returns the list", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => json({ sessions: [session()] }));
    expect(await fetchSessions(BASE, "tok", { fetchImpl })).toEqual([session()]);
    expect(fetchImpl.mock.calls[0]?.[0]).toBe(`${BASE}/v1/me/sessions`);
  });

  it.each([
    ["401", async () => json({}, { status: 401 })],
    ["a bad shape", async () => json({ sessions: [{ id: 1 }] })],
    ["a failure", async () => Promise.reject(new Error("x"))],
  ])("returns null for %s", async (_name, impl) => {
    expect(await fetchSessions(BASE, "tok", { fetchImpl: vi.fn<typeof fetch>(impl) })).toBeNull();
  });
});

describe("fetchProviders", () => {
  it("returns the configured providers and sends no cookie", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => json({ providers: ["google", "github"] }));
    expect(await fetchProviders(BASE, { fetchImpl })).toEqual(["google", "github"]);
    expect(fetchImpl.mock.calls[0]?.[0]).toBe(`${BASE}/v1/auth/providers`);
    const init = fetchImpl.mock.calls[0]?.[1];
    expect(new Headers(init?.headers).has("cookie")).toBe(false);
    expect(init?.redirect).toBe("error");
    expect(init?.cache).toBe("no-store");
  });

  it("returns none when none are configured", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => json({ providers: [] }));
    expect(await fetchProviders(BASE, { fetchImpl })).toEqual([]);
  });

  it.each([
    ["an unreachable API", () => Promise.reject(new Error("ECONNREFUSED"))],
    ["a server error", async () => json({ error: "internal_error" }, { status: 500 })],
    ["an unknown provider", async () => json({ providers: ["facebook"] })],
    ["too many providers", async () => json({ providers: ["google", "github", "google"] })],
    ["the wrong shape", async () => json({ providers: "google" })],
    ["a body that is not JSON", async () => new Response("<html>", { status: 200 })],
    ["a redirect", () => Promise.reject(new TypeError("redirect"))],
  ])("shows no buttons for %s (email codes still work)", async (_name, impl) => {
    const fetchImpl = vi.fn<typeof fetch>(impl as () => Promise<Response>);
    expect(await fetchProviders(BASE, { fetchImpl })).toEqual([]);
  });

  it("gives up on a slow API", async () => {
    const fetchImpl = vi.fn<typeof fetch>(
      (_u, init) =>
        new Promise((_res, rej) =>
          init?.signal?.addEventListener("abort", () => rej(new Error("t"))),
        ),
    );
    expect(await fetchProviders(BASE, { fetchImpl, timeoutMs: 20 })).toEqual([]);
  });
});

describe("fetchIdentities", () => {
  it("returns the connected accounts and sends the session cookie", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => json({ identities: [identity()] }));
    expect(await fetchIdentities(BASE, "tok", { fetchImpl })).toEqual([identity()]);
    expect(fetchImpl.mock.calls[0]?.[0]).toBe(`${BASE}/v1/me/identities`);
    expect(new Headers(fetchImpl.mock.calls[0]?.[1]?.headers).get("cookie")).toBe(
      "__Host-rm_session=tok",
    );
  });

  it.each([
    ["a 401", async () => json({ error: "unauthenticated" }, { status: 401 })],
    ["a server error", async () => json({ error: "internal_error" }, { status: 500 })],
    ["the wrong shape", async () => json({ identities: [{ provider: "x" }] })],
  ])("is null for %s", async (_name, impl) => {
    const fetchImpl = vi.fn<typeof fetch>(impl);
    expect(await fetchIdentities(BASE, "tok", { fetchImpl })).toBeNull();
  });
});
