import { expect, test, type Browser, type Page } from "@playwright/test";
import { freshEmail, latestCode, resetRateLimits } from "./stack";

const SESSION_COOKIE = "__Host-rm_session";

test.beforeAll(() => resetRateLimits());

async function signIn(page: Page, email: string) {
  await page.goto("/login");
  await page.getByLabel("Email address").fill(email);
  await page.getByRole("button", { name: "Send code" }).click();
  await expect(page.getByLabel("6-digit code")).toBeVisible();
  const code = await latestCode(email);
  await page.getByLabel("6-digit code").fill(code);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  // An emailed code lands on the dashboard; the account page is one click away.
  await expect(page).toHaveURL(/\/dashboard$/);
  await page.goto("/account");
  await expect(page).toHaveURL(/\/account$/);
}

async function newSignedInContext(browser: Browser, email: string) {
  const context = await browser.newContext();
  const page = await context.newPage();
  await signIn(page, email);
  return { context, page };
}

test.describe("signed out", () => {
  test("/account sends a signed-out visitor to /login before showing anything", async ({
    page,
  }) => {
    const response = await page.goto("/account");
    await expect(page).toHaveURL(/\/login$/);
    expect(response?.status()).toBe(200);
    await expect(
      page.getByRole("heading", { name: /sign in or create an account/i }),
    ).toBeVisible();
    await expect(page.getByText("Your account")).toHaveCount(0);
  });

  test("a forged or stale cookie is no better than none", async ({ page, context, baseURL }) => {
    await context.addCookies([
      {
        name: SESSION_COOKIE,
        value: "A".repeat(43),
        domain: new URL(baseURL!).hostname,
        path: "/",
        secure: true,
        httpOnly: true,
        sameSite: "Lax",
      },
    ]);
    await page.goto("/account");
    await expect(page).toHaveURL(/\/login$/);
  });

  test("the bridge forwards the account API and nothing else (audit F14)", async ({ request }) => {
    for (const path of [
      "/api/readyz",
      "/api/healthz",
      "/api/api/v1/status",
      "/api/v1/nope",
      "/api/v1/me/..",
    ]) {
      const res = await request.get(path);
      expect(res.status(), path).toBe(404);
      const text = await res.text();
      expect(text, path).not.toMatch(/postgres|redis|ready|operational/i);
    }
    const me = await request.get("/api/v1/me");
    expect(me.status()).toBe(401);
    expect(await me.json()).toMatchObject({ error: "unauthenticated" });
  });

  test("a request from another origin is refused", async ({ request }) => {
    const res = await request.post("/api/v1/auth/email/start", {
      headers: { origin: "https://evil.example" },
      data: { email: freshEmail("csrf") },
    });
    expect(res.status()).toBe(403);
    expect(await res.json()).toMatchObject({ error: "forbidden" });
  });

  test("a wrong code is refused, with one message, and the right one then works", async ({
    page,
  }) => {
    const email = freshEmail("wrong");
    await page.goto("/login");
    await page.getByLabel("Email address").fill(email);
    await page.getByRole("button", { name: "Send code" }).click();
    const code = await latestCode(email);
    const wrong = code === "000000" ? "111111" : "000000";

    await page.getByLabel("6-digit code").fill(wrong);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    // Next.js adds its own route announcer with role=alert, so address ours by id.
    await expect(page.locator("#form-error")).toContainText(/wrong or has expired/i);
    await expect(page).toHaveURL(/\/login$/);

    await page.getByLabel("6-digit code").fill(code);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page).toHaveURL(/\/dashboard$/);
  });
});

test.describe("signing in", () => {
  test("full flow: sign in, cookie, reload, rename, sign out", async ({ page, context }) => {
    const email = freshEmail("flow");
    // The pages must run under the strict CSP without a single violation.
    const violations: string[] = [];
    page.on("console", (m) => {
      if (/content.security.policy/i.test(m.text())) violations.push(m.text());
    });
    await signIn(page, email);
    await expect(page.getByRole("main").getByText(email)).toBeVisible();

    // Scenarios 11 and 20: Chromium accepts the __Host- cookie over http://127.0.0.1.
    const cookies = await context.cookies();
    const cookie = cookies.find((c) => c.name === SESSION_COOKIE);
    expect(cookie, "the session cookie was not stored").toBeDefined();
    expect(cookie).toMatchObject({ secure: true, httpOnly: true, sameSite: "Lax", path: "/" });
    expect(cookie?.value).toHaveLength(43);

    // Scripts cannot read it.
    expect(await page.evaluate(() => document.cookie)).not.toContain(SESSION_COOKIE);

    // The session survives a reload; the signed-in user is not sent to /login.
    await page.reload();
    await expect(page).toHaveURL(/\/account$/);
    await expect(page.getByRole("main").getByText(email)).toBeVisible();
    await page.goto("/login");
    await expect(page).toHaveURL(/\/dashboard$/);
    await page.goto("/account");

    // Rename: persists across a reload.
    await page.getByLabel("Display name").fill("Ada Lovelace");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await expect(page.getByRole("status").filter({ hasText: "Saved." })).toBeVisible();
    await page.reload();
    await expect(page.getByLabel("Display name")).toHaveValue("Ada Lovelace");

    // This device is listed and marked.
    const item = page.getByRole("list", { name: "Active sessions" }).getByRole("listitem");
    await expect(item).toHaveCount(1);
    await expect(item.getByText("This device", { exact: true })).toBeVisible();

    // Sign out: back at /login, cookie gone, /account is closed again.
    await page.getByRole("button", { name: "Sign out of this device" }).click();
    await expect(page).toHaveURL(/\/login$/);
    expect((await context.cookies()).find((c) => c.name === SESSION_COOKIE)).toBeUndefined();
    await page.goto("/account");
    await expect(page).toHaveURL(/\/login$/);
    expect(violations).toEqual([]);
  });

  test("a second device is listed, and signing it out ends it at once", async ({ browser }) => {
    const email = freshEmail("devices");
    const first = await newSignedInContext(browser, email);
    resetRateLimits(); // the same address may only ask for one code a minute
    const second = await newSignedInContext(browser, email);

    await first.page.reload();
    const items = first.page.getByRole("list", { name: "Active sessions" }).getByRole("listitem");
    await expect(items).toHaveCount(2);
    await expect(first.page.getByText("This device", { exact: true })).toHaveCount(1);

    await first.page.getByRole("button", { name: /^Sign out .*signed in/ }).click();
    await expect(items).toHaveCount(1);

    // The other device is signed out on its next request.
    await second.page.reload();
    await expect(second.page).toHaveURL(/\/login$/);
    // This one is untouched.
    await first.page.reload();
    await expect(first.page).toHaveURL(/\/account$/);

    await first.context.close();
    await second.context.close();
  });

  test("the account API refuses a change without the page's CSRF token", async ({ page }) => {
    await signIn(page, freshEmail("csrf2"));
    // Same browser, same cookie, but no token: what a forged cross-site request would look like.
    const res = await page.request.patch("/api/v1/me", {
      data: { display_name: "Hacked" },
      headers: { origin: new URL(page.url()).origin },
    });
    expect(res.status()).toBe(403);
    await page.reload();
    await expect(page.getByLabel("Display name")).not.toHaveValue("Hacked");
  });
});
