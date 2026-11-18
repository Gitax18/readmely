import { describe, expect, it, vi } from "vitest";
import { MAX_REQUEST_BYTES, MAX_RESPONSE_BYTES, proxyToApi, type ProxyOptions } from "./api-proxy";

const SESSION_ID = "01900000-0000-7000-8000-000000000401";

function upstream(body: string | null, init: ResponseInit = {}): Response {
  return new Response(body, {
    status: 200,
    headers: { "content-type": "application/json" },
    ...init,
  });
}

function setup(response: Response | Error = upstream("{}"), extra: Partial<ProxyOptions> = {}) {
  const fetchImpl = vi.fn<typeof fetch>(async () => {
    if (response instanceof Error) throw response;
    return response;
  });
  const options: ProxyOptions = {
    apiPublicUrl: "http://api:8080",
    behindProxy: false,
    fetchImpl,
    ...extra,
  };
  return { fetchImpl, options };
}

const call = (path: string[], init: RequestInit & { headers?: Record<string, string> } = {}) =>
  new Request(`http://web.test/api/${path.join("/")}`, init);

describe("proxyToApi: which paths are forwarded", () => {
  it.each([
    ["POST", ["v1", "auth", "email", "start"]],
    ["POST", ["v1", "auth", "email", "verify"]],
    ["POST", ["v1", "auth", "logout"]],
    ["GET", ["v1", "me"]],
    ["PATCH", ["v1", "me"]],
    ["GET", ["v1", "me", "sessions"]],
    ["DELETE", ["v1", "me", "sessions", SESSION_ID]],
  ])("forwards %s /%s", async (method, path) => {
    const { fetchImpl, options } = setup();
    const res = await proxyToApi(
      call(path, { method, ...(method === "GET" ? {} : { body: "{}" }) }),
      path,
      options,
    );
    expect(res.status).toBe(200);
    expect(fetchImpl).toHaveBeenCalledOnce();
    expect(fetchImpl.mock.calls[0]?.[0]).toBe(`http://api:8080/${path.join("/")}`);
  });

  // Audit F14: the internal listener's paths must not be reachable from the browser.
  it.each([
    ["the readiness probe", ["readyz"]],
    ["the readiness probe under /v1", ["v1", "readyz"]],
    ["the status detail", ["api", "v1", "status"]],
    ["liveness", ["healthz"]],
    ["nothing", []],
    ["a prefix only", ["v1"]],
    ["an extra segment", ["v1", "me", "extra"]],
    ["an extra segment after start", ["v1", "auth", "email", "start", "x"]],
    ["a dot-dot escape", ["v1", "me", "..", "..", "readyz"]],
    ["a dot-dot after v1", ["v1", "..", "readyz"]],
    ["an encoded slash", ["v1", "me%2f..%2freadyz"]],
    ["a session id that is not a UUID", ["v1", "me", "sessions", "abc"]],
    ["a session id with a suffix", ["v1", "me", "sessions", `${SESSION_ID}x`]],
    ["a session id with a newline", ["v1", "me", "sessions", `${SESSION_ID}\n`]],
    ["a different case of a known path", ["V1", "me"]],
    ["an empty segment", ["v1", "", "me"]],
  ])("refuses %s without calling the API", async (_name, path) => {
    const { fetchImpl, options } = setup();
    const res = await proxyToApi(call(path.length ? path : ["x"]), path, options);
    expect(res.status).toBe(404);
    expect(await res.json()).toMatchObject({ error: "not_found" });
    expect(fetchImpl).not.toHaveBeenCalled();
  });
});

describe("proxyToApi: what is forwarded", () => {
  it("passes only the headers the API needs", async () => {
    const { fetchImpl, options } = setup();
    const path = ["v1", "me"];
    await proxyToApi(
      call(path, {
        method: "PATCH",
        body: "{}",
        headers: {
          cookie: "__Host-rm_session=abc",
          origin: "http://127.0.0.1:3000",
          "content-type": "application/json",
          "x-csrf-token": "c".repeat(64),
          "user-agent": "TestBrowser/1",
          accept: "application/json",
          authorization: "Bearer secret",
          "x-api-key": "secret",
          host: "evil.example",
          "x-forwarded-host": "evil.example",
          "x-forwarded-for": "1.2.3.4",
          "x-real-ip": "1.2.3.4",
        },
      }),
      path,
      options,
    );
    const init = fetchImpl.mock.calls[0]?.[1];
    const sent = Object.fromEntries(new Headers(init?.headers).entries());
    expect(sent).toEqual({
      cookie: "__Host-rm_session=abc",
      origin: "http://127.0.0.1:3000",
      "content-type": "application/json",
      "x-csrf-token": "c".repeat(64),
      "user-agent": "TestBrowser/1",
      accept: "application/json",
    });
    expect(init?.method).toBe("PATCH");
    expect(init?.redirect).toBe("manual");
    expect(init?.cache).toBe("no-store");
  });

  it("drops X-Forwarded-For unless the server is behind a trusted proxy", async () => {
    const path = ["v1", "auth", "email", "start"];
    const headers = { "x-forwarded-for": "203.0.113.7", "content-type": "application/json" };

    const direct = setup();
    await proxyToApi(call(path, { method: "POST", body: "{}", headers }), path, direct.options);
    expect(new Headers(direct.fetchImpl.mock.calls[0]?.[1]?.headers).has("x-forwarded-for")).toBe(
      false,
    );

    const proxied = setup(upstream("{}"), { behindProxy: true });
    await proxyToApi(call(path, { method: "POST", body: "{}", headers }), path, proxied.options);
    expect(new Headers(proxied.fetchImpl.mock.calls[0]?.[1]?.headers).get("x-forwarded-for")).toBe(
      "203.0.113.7",
    );
  });

  it("sends the request body, and none on GET", async () => {
    const { fetchImpl, options } = setup();
    const start = ["v1", "auth", "email", "start"];
    await proxyToApi(
      call(start, { method: "POST", body: '{"email":"a@example.com"}' }),
      start,
      options,
    );
    const body = fetchImpl.mock.calls[0]?.[1]?.body as ArrayBuffer;
    expect(new TextDecoder().decode(body)).toBe('{"email":"a@example.com"}');

    const me = ["v1", "me"];
    await proxyToApi(call(me), me, options);
    expect(fetchImpl.mock.calls[1]?.[1]?.body).toBeUndefined();
  });

  it("refuses a request body over the limit, declared or not", async () => {
    const { fetchImpl, options } = setup();
    const path = ["v1", "me"];
    const big = "x".repeat(MAX_REQUEST_BYTES + 1);
    const declared = await proxyToApi(
      call(path, { method: "PATCH", body: big, headers: { "content-length": String(big.length) } }),
      path,
      options,
    );
    expect(declared.status).toBe(413);
    const undeclared = await proxyToApi(
      new Request("http://web.test/api/v1/me", { method: "PATCH", body: big }),
      path,
      options,
    );
    expect(undeclared.status).toBe(413);
    expect(fetchImpl).not.toHaveBeenCalled();
  });
});

describe("proxyToApi: what comes back", () => {
  it("passes status, body and the headers the browser needs, and nothing else", async () => {
    const response = upstream('{"error":"rate_limited","request_id":"abc12345"}', {
      status: 429,
      headers: {
        "content-type": "application/json",
        "retry-after": "42",
        "x-request-id": "abc12345",
        server: "secret-go-version",
        "content-security-policy": "default-src 'none'",
        "access-control-allow-origin": "*",
        location: "http://internal:8081/readyz",
      },
    });
    const { options } = setup(response);
    const path = ["v1", "auth", "email", "start"];
    const res = await proxyToApi(call(path, { method: "POST", body: "{}" }), path, options);
    expect(res.status).toBe(429);
    expect(await res.text()).toBe('{"error":"rate_limited","request_id":"abc12345"}');
    expect(res.headers.get("retry-after")).toBe("42");
    expect(res.headers.get("x-request-id")).toBe("abc12345");
    expect(res.headers.get("content-type")).toBe("application/json");
    expect(res.headers.get("cache-control")).toBe("no-store");
    for (const leaked of [
      "server",
      "content-security-policy",
      "access-control-allow-origin",
      "location",
    ]) {
      expect(res.headers.has(leaked)).toBe(false);
    }
  });

  it("keeps every Set-Cookie header intact", async () => {
    const headers = new Headers({ "content-type": "application/json" });
    headers.append(
      "set-cookie",
      "__Host-rm_session=tok; Path=/; Secure; HttpOnly; SameSite=Lax; Max-Age=2592000",
    );
    headers.append("set-cookie", "other=1; Path=/");
    const { options } = setup(new Response("{}", { status: 200, headers }));
    const path = ["v1", "auth", "email", "verify"];
    const res = await proxyToApi(call(path, { method: "POST", body: "{}" }), path, options);
    expect(res.headers.getSetCookie()).toEqual([
      "__Host-rm_session=tok; Path=/; Secure; HttpOnly; SameSite=Lax; Max-Age=2592000",
      "other=1; Path=/",
    ]);
  });

  it("answers 204 with no body", async () => {
    const { options } = setup(new Response(null, { status: 204 }));
    const path = ["v1", "auth", "logout"];
    const res = await proxyToApi(call(path, { method: "POST" }), path, options);
    expect(res.status).toBe(204);
    expect(await res.text()).toBe("");
  });

  it("does not follow or expose a redirect", async () => {
    const { options } = setup(
      new Response(null, { status: 302, headers: { location: "http://internal/" } }),
    );
    const path = ["v1", "me"];
    const res = await proxyToApi(call(path), path, options);
    expect(res.status).toBe(302);
    expect(res.headers.has("location")).toBe(false);
  });

  it("turns an unreachable API into a generic 502", async () => {
    const { options } = setup(new Error("connect ECONNREFUSED 10.0.0.5:8080"));
    const path = ["v1", "me"];
    const res = await proxyToApi(call(path), path, options);
    expect(res.status).toBe(502);
    const text = await res.text();
    expect(JSON.parse(text)).toMatchObject({ error: "bad_gateway" });
    expect(text).not.toContain("10.0.0.5");
  });

  it("gives up on a response over the limit", async () => {
    const { options } = setup(upstream("x".repeat(MAX_RESPONSE_BYTES + 1)));
    const path = ["v1", "me"];
    const res = await proxyToApi(call(path), path, options);
    expect(res.status).toBe(502);
  });

  it("times out a hung API", async () => {
    const fetchImpl = vi.fn<typeof fetch>(
      (_url, init) =>
        new Promise((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => reject(new Error("aborted")));
        }),
    );
    const path = ["v1", "me"];
    const res = await proxyToApi(call(path), path, {
      apiPublicUrl: "http://api:8080",
      behindProxy: false,
      fetchImpl,
      timeoutMs: 20,
    });
    expect(res.status).toBe(502);
  });
});
