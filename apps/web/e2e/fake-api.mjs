// A stand-in for the Go API's internal listener, for browser tests only.
// Scenario is switched with POST /__scenario/<name>; GET /api/v1/status answers per scenario.
import http from "node:http";

const ok = (api = "ok", ai = "ok", overall = "operational") => ({
  status: overall,
  services: {
    api: { status: api, checks: { postgres: api === "ok" ? "ok" : "down", redis: "ok" } },
    ai: { status: ai, checks: ai === "ok" ? { postgres: "ok" } : {} },
  },
});

const scenarios = {
  ok: (res) => json(res, 200, ok()),
  degraded: (res) => json(res, 200, ok("ok", "unreachable", "degraded")),
  unavailable: (res) => json(res, 200, ok("down", "unreachable", "unavailable")),
  error500: (res) => json(res, 500, { error: "internal", request_id: "x" }),
  malformed: (res) => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end("<html>not json</html>");
  },
  // Never answers: the web app must time out and still render.
  hang: () => {},
  // Hostile values in every string field.
  xss: (res) =>
    json(res, 200, {
      status: "<img src=x onerror=window.__pwned=1>",
      services: {
        api: { status: "<script>window.__pwned=1</script>", checks: {} },
        ai: { status: "ok", checks: {} },
      },
    }),
  huge: (res) => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ pad: "a".repeat(200 * 1024) }));
  },
};

function json(res, code, body) {
  res.writeHead(code, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

let current = "ok";

http
  .createServer((req, res) => {
    const url = new URL(req.url ?? "/", "http://localhost");
    if (req.method === "POST" && url.pathname.startsWith("/__scenario/")) {
      const name = url.pathname.slice("/__scenario/".length);
      if (!(name in scenarios)) return json(res, 404, { error: "unknown scenario" });
      current = name;
      return json(res, 200, { scenario: name });
    }
    if (req.method === "GET" && url.pathname === "/__health") return json(res, 200, { ok: true });
    if (req.method === "GET" && url.pathname === "/api/v1/status") return scenarios[current](res);
    return json(res, 404, { error: "not_found" });
  })
  .listen(Number(process.env.FAKE_API_PORT ?? 4010), "127.0.0.1");
