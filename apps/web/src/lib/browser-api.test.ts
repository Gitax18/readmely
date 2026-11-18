import { describe, expect, it, vi } from "vitest";
import { CSRF, json, me, session } from "@/test/account-fixtures";
import { createAccountApi } from "./browser-api";

function api(impl: () => Promise<Response>) {
  const fetchImpl = vi.fn<typeof fetch>(impl);
  return { api: createAccountApi(fetchImpl), fetchImpl };
}

const err = (status: number, error: string, headers: Record<string, string> = {}) =>
  json({ error, request_id: "abc12345" }, { status, headers });

describe("createAccountApi: requests", () => {
  it("startLogin posts the email as JSON to the same-origin bridge", async () => {
    const { api: a, fetchImpl } = api(async () => json({ status: "accepted" }, { status: 202 }));
    expect(await a.startLogin("ada@example.com")).toEqual({ ok: true });
    const [url, init] = fetchImpl.mock.calls[0] ?? [];
    expect(url).toBe("/api/v1/auth/email/start");
    expect(init?.method).toBe("POST");
    expect(init?.credentials).toBe("same-origin");
    expect(init?.cache).toBe("no-store");
    expect(JSON.parse(String(init?.body))).toEqual({ email: "ada@example.com" });
    const h = new Headers(init?.headers);
    expect(h.get("content-type")).toBe("application/json");
    expect(h.has("x-csrf-token")).toBe(false);
  });

  it("verifyLogin posts email and code and returns the user", async () => {
    const { api: a, fetchImpl } = api(async () => json(me()));
    const r = await a.verifyLogin("ada@example.com", "013842");
    expect(r).toEqual({ ok: true, me: me() });
    expect(JSON.parse(String(fetchImpl.mock.calls[0]?.[1]?.body))).toEqual({
      email: "ada@example.com",
      code: "013842",
    });
  });

  it("state-changing calls carry the CSRF token", async () => {
    const { api: a, fetchImpl } = api(async () => new Response(null, { status: 204 }));
    await a.logout(CSRF);
    await a.revokeSession(CSRF, "01900000-0000-7000-8000-000000000402");
    for (const call of fetchImpl.mock.calls) {
      expect(new Headers(call[1]?.headers).get("x-csrf-token")).toBe(CSRF);
    }
    expect(fetchImpl.mock.calls[0]?.[0]).toBe("/api/v1/auth/logout");
    expect(fetchImpl.mock.calls[1]?.[0]).toBe(
      "/api/v1/me/sessions/01900000-0000-7000-8000-000000000402",
    );
    expect(fetchImpl.mock.calls[1]?.[1]?.method).toBe("DELETE");
  });

  it("encodes a session id so it cannot change the path", async () => {
    const { api: a, fetchImpl } = api(async () => new Response(null, { status: 204 }));
    await a.revokeSession(CSRF, "../../me");
    expect(fetchImpl.mock.calls[0]?.[0]).toBe("/api/v1/me/sessions/..%2F..%2Fme");
  });

  it("rename patches the display name", async () => {
    const { api: a, fetchImpl } = api(async () => json(me({ display_name: "Ada" }).user));
    const r = await a.rename(CSRF, "Ada");
    expect(r).toEqual({ ok: true, user: me({ display_name: "Ada" }).user });
    expect(fetchImpl.mock.calls[0]?.[1]?.method).toBe("PATCH");
    expect(JSON.parse(String(fetchImpl.mock.calls[0]?.[1]?.body))).toEqual({ display_name: "Ada" });
  });

  it("listSessions returns the sessions", async () => {
    const { api: a } = api(async () => json({ sessions: [session()] }));
    expect(await a.listSessions()).toEqual({ ok: true, sessions: [session()] });
  });
});

describe("createAccountApi: failures", () => {
  it("maps a 429 to rate_limited with Retry-After", async () => {
    const { api: a } = api(async () => err(429, "rate_limited", { "retry-after": "37" }));
    expect(await a.startLogin("a@example.com")).toEqual({
      ok: false,
      kind: "rate_limited",
      retryAfterSeconds: 37,
    });
  });

  it.each([
    ["missing", {}],
    ["junk", { "retry-after": "soon" }],
    ["zero", { "retry-after": "0" }],
  ])("falls back to 60 seconds when Retry-After is %s", async (_n, headers) => {
    const { api: a } = api(async () => err(429, "rate_limited", headers));
    expect(await a.startLogin("a@example.com")).toMatchObject({
      kind: "rate_limited",
      retryAfterSeconds: 60,
    });
  });

  it("tells a wrong code from a malformed request", async () => {
    expect(
      await api(async () => err(400, "invalid_code")).api.verifyLogin("a@example.com", "123456"),
    ).toEqual({
      ok: false,
      kind: "invalid_code",
    });
    expect(await api(async () => err(400, "invalid_request")).api.startLogin("nope")).toEqual({
      ok: false,
      kind: "invalid_request",
    });
  });

  it.each([
    [401, "unauthenticated", "unauthenticated"],
    [403, "forbidden", "forbidden"],
    [404, "not_found", "not_found"],
    [500, "internal_error", "unavailable"],
    [502, "bad_gateway", "unavailable"],
  ])("maps %i to %s", async (status, code, kind) => {
    expect(await api(async () => err(status, code)).api.logout(CSRF)).toEqual({ ok: false, kind });
  });

  it("copes with an error body that is not JSON", async () => {
    const { api: a } = api(async () => new Response("<html>bad gateway</html>", { status: 502 }));
    expect(await a.logout(CSRF)).toEqual({ ok: false, kind: "unavailable" });
    const { api: b } = api(async () => new Response("oops", { status: 400 }));
    expect(await b.startLogin("a@example.com")).toEqual({ ok: false, kind: "invalid_request" });
  });

  it("maps a network failure to unavailable", async () => {
    const { api: a } = api(async () => Promise.reject(new TypeError("Failed to fetch")));
    expect(await a.startLogin("a@example.com")).toEqual({ ok: false, kind: "unavailable" });
  });

  it.each([
    [
      "verifyLogin",
      (a: ReturnType<typeof createAccountApi>) => a.verifyLogin("a@example.com", "123456"),
    ],
    ["rename", (a: ReturnType<typeof createAccountApi>) => a.rename(CSRF, "Ada")],
    ["listSessions", (a: ReturnType<typeof createAccountApi>) => a.listSessions()],
  ])("%s does not trust a success body of the wrong shape", async (_n, run) => {
    const { api: a } = api(async () => json({ unexpected: true }));
    expect(await run(a)).toEqual({ ok: false, kind: "unavailable" });
    const { api: b } = api(async () => new Response("not json", { status: 200 }));
    expect(await run(b)).toEqual({ ok: false, kind: "unavailable" });
  });

  it("an unexpected success status is not success", async () => {
    // start must be exactly 202; a 200 from a captive portal or a misrouted page is not.
    const { api: a } = api(async () => json({ status: "accepted" }, { status: 200 }));
    expect(await a.startLogin("a@example.com")).toEqual({ ok: false, kind: "unavailable" });
  });
});
