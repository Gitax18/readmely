import { expect, test } from "@playwright/test";

const FAKE_API = "http://127.0.0.1:4010";
const COARSE = "http://127.0.0.1:3101";

async function scenario(name: string): Promise<void> {
  const res = await fetch(`${FAKE_API}/__scenario/${name}`, { method: "POST" });
  expect(res.status).toBe(200);
}

test.beforeEach(async () => {
  await scenario("ok");
});

test.describe("status page", () => {
  test("loads and shows everything operational", async ({ page }) => {
    await page.goto("/status");
    await expect(page).toHaveTitle("Readmely status");
    await expect(page.getByRole("heading", { level: 1, name: "Readmely status" })).toBeVisible();
    await expect(
      page.getByRole("heading", { level: 2, name: "All systems operational" }),
    ).toBeVisible();
    await expect(page.getByText("API", { exact: true })).toBeVisible();
    await expect(page.getByText("AI service")).toBeVisible();
  });

  test("shows each dependency at the full detail level", async ({ page }) => {
    await page.goto("/status");
    await expect(page.getByText("postgres", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("redis", { exact: true })).toBeVisible();
  });

  test("shows degraded when the AI service is unreachable", async ({ page }) => {
    await scenario("degraded");
    await page.goto("/status");
    await expect(
      page.getByRole("heading", { level: 2, name: "Some services are degraded" }),
    ).toBeVisible();
    await expect(page.getByText("Unreachable")).toBeVisible();
  });

  test("shows unavailable when the API's own database is down", async ({ page }) => {
    await scenario("unavailable");
    await page.goto("/status");
    await expect(
      page.getByRole("heading", { level: 2, name: "Service unavailable" }),
    ).toBeVisible();
  });

  test("shows API unreachable when the API errors", async ({ page }) => {
    await scenario("error500");
    await page.goto("/status");
    await expect(
      page.getByRole("heading", { level: 2, name: /can.t reach the readmely api/i }),
    ).toBeVisible();
  });

  test("times out a hanging API and still renders", async ({ page }) => {
    await scenario("hang");
    const start = Date.now();
    await page.goto("/status");
    await expect(
      page.getByRole("heading", { level: 2, name: /can.t reach the readmely api/i }),
    ).toBeVisible();
    expect(Date.now() - start).toBeLessThan(8000);
  });

  test("refuses malformed and oversized API responses", async ({ page }) => {
    for (const name of ["malformed", "huge"]) {
      await scenario(name);
      await page.goto("/status");
      await expect(
        page.getByRole("heading", { level: 2, name: /invalid response/i }),
      ).toBeVisible();
    }
  });

  test("hostile API values are never executed or rendered", async ({ page }) => {
    await scenario("xss");
    const dialogs: string[] = [];
    page.on("dialog", (d) => dialogs.push(d.message()));
    await page.goto("/status");
    await expect(page.getByRole("heading", { level: 2, name: /invalid response/i })).toBeVisible();
    expect(
      await page.evaluate(() => (window as unknown as { __pwned?: number }).__pwned),
    ).toBeUndefined();
    expect(dialogs).toEqual([]);
    expect(await page.content()).not.toContain("onerror=window.__pwned");
  });

  test("has sensible semantics and conveys status in text", async ({ page }) => {
    await page.goto("/status");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await expect(page.getByRole("main")).toHaveCount(1);
    await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
    // Every service state is spelled out, not just coloured.
    await expect(page.getByText("Operational", { exact: true }).first()).toBeVisible();
  });

  test("/healthz answers without touching the API", async ({ request }) => {
    await scenario("hang");
    const res = await request.get("/healthz");
    expect(res.status()).toBe(200);
    expect(await res.json()).toEqual({ status: "ok" });
  });
});

test.describe("coarse detail (production default)", () => {
  test("shows overall and per-service state but no dependency names, in the HTML or the DOM", async ({
    page,
    request,
  }) => {
    await page.goto(`${COARSE}/status`);
    await expect(
      page.getByRole("heading", { level: 2, name: "All systems operational" }),
    ).toBeVisible();
    await expect(page.getByText("AI service")).toBeVisible();

    const raw = await (await request.get(`${COARSE}/status`)).text();
    expect(raw).not.toMatch(/postgres|redis/i);
    expect(await page.content()).not.toMatch(/postgres|redis/i);
  });
});

test.describe("security headers", () => {
  test("sends the security headers and no X-Powered-By", async ({ request }) => {
    const h = (await request.get("/")).headers();
    expect(h["x-content-type-options"]).toBe("nosniff");
    expect(h["x-frame-options"]).toBe("DENY");
    expect(h["referrer-policy"]).toBe("strict-origin-when-cross-origin");
    expect(h["permissions-policy"]).toContain("camera=()");
    expect(h["cross-origin-opener-policy"]).toBe("same-origin");
    expect(h["x-powered-by"]).toBeUndefined();
    expect(h["cache-control"]).toContain("no-store");
  });

  test("CSP is strict and its nonce changes on every request", async ({ request }) => {
    const nonce = (csp: string) => /'nonce-([^']+)'/.exec(csp)?.[1];
    const a = (await request.get("/")).headers()["content-security-policy"] ?? "";
    const b = (await request.get("/")).headers()["content-security-policy"] ?? "";
    expect(nonce(a)).toBeTruthy();
    expect(nonce(a)).not.toBe(nonce(b));
    expect(a).toContain("frame-ancestors 'none'");
    expect(a).toContain("object-src 'none'");
    expect(a).not.toMatch(/unsafe-inline|unsafe-eval/);
  });

  test("every script on the page carries the request's nonce", async ({ request }) => {
    const res = await request.get("/");
    const nonce = /'nonce-([^']+)'/.exec(res.headers()["content-security-policy"] ?? "")?.[1];
    const html = await res.text();
    const scripts = html.match(/<script\b[^>]*>/g) ?? [];
    expect(scripts.length).toBeGreaterThan(0);
    for (const tag of scripts) expect(tag).toContain(`nonce="${nonce}"`);
  });

  test("the page runs under its own CSP without violations", async ({ page }) => {
    const problems: string[] = [];
    page.on("console", (m) => {
      if (m.type() === "error") problems.push(m.text());
    });
    page.on("pageerror", (e) => problems.push(e.message));

    await page.goto("/status");
    await expect(
      page.getByRole("heading", { level: 2, name: "All systems operational" }),
    ).toBeVisible();
    expect(problems).toEqual([]);
  });

  // 'strict-dynamic' deliberately trusts scripts created by already-trusted scripts,
  // so a script made through the DOM API is allowed. What an XSS attack really produces
  // is markup in the HTML stream (parser-inserted), and that must be refused.
  test("blocks an injected inline script that has no nonce", async ({ page }) => {
    const messages: string[] = [];
    page.on("console", (m) => messages.push(m.text()));

    await page.goto("/");
    await page.evaluate(() => {
      document.open();
      document.write("<script>window.__injected = true</script><p>after</p>");
      document.close();
    });
    await expect(page.getByText("after")).toBeVisible();

    expect(
      await page.evaluate(() => (window as unknown as { __injected?: boolean }).__injected),
    ).toBeUndefined();
    expect(messages.some((m) => /Content Security Policy/i.test(m))).toBe(true);
  });
});
