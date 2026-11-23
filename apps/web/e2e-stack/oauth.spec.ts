import { expect, test, type Page } from "@playwright/test";
import { freshEmail, identityCount, latestCode, resetRateLimits, seedIdentity } from "./stack";

// Provider sign-in against the real stack. There is no provider to approve a sign-in in a test, so
// the round trip itself is covered by the Go tests against a fake provider; these tests cover
// everything the browser and the same-origin bridge add around it. Expectations come from what
// the API says is configured, so the same tests pass with no providers, with the CI's throwaway
// Google client, or with real ones.

const ALL = ["google", "github"] as const;
type Provider = (typeof ALL)[number];

const AUTHORIZATION_HOST: Record<Provider, string> = {
  google: "accounts.google.com",
  github: "github.com",
};

test.beforeAll(() => resetRateLimits());

// Next.js keeps its own empty role=alert element for route announcements; ours are the others.
const alerts = (page: Page) => page.locator('[role="alert"]:not(#__next-route-announcer__)');

async function configured(
  request: import("@playwright/test").APIRequestContext,
): Promise<Provider[]> {
  const res = await request.get("/api/v1/auth/providers");
  expect(res.status()).toBe(200);
  const body = (await res.json()) as { providers: Provider[] };
  return body.providers;
}

async function signIn(page: Page, email: string) {
  await page.goto("/login");
  await page.getByLabel("Email address").fill(email);
  await page.getByRole("button", { name: "Send code" }).click();
  await expect(page.getByLabel("6-digit code")).toBeVisible();
  await page.getByLabel("6-digit code").fill(await latestCode(email));
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page).toHaveURL(/\/account$/);
}

test.describe("the login page and the bridge", () => {
  test("shows a button for each configured provider, and for no other", async ({
    page,
    request,
  }) => {
    const providers = await configured(request);
    await page.goto("/login");
    const links = page.getByRole("link", { name: /^continue with /i });
    await expect(links).toHaveCount(providers.length);
    for (const p of providers) {
      await expect(page.locator(`a[href="/api/v1/auth/${p}/start"]`)).toBeVisible();
    }
    // The email form is always there.
    await expect(page.getByLabel("Email address")).toBeVisible();
  });

  test("an unknown or unconfigured provider is not found, on start and on callback", async ({
    request,
  }) => {
    const providers = await configured(request);
    const missing = [...ALL.filter((p) => !providers.includes(p)), "facebook", "email", "GOOGLE"];
    for (const p of missing) {
      for (const step of ["start", "callback"]) {
        const res = await request.get(`/api/v1/auth/${p}/${step}?code=x&state=y`, {
          maxRedirects: 0,
        });
        expect(res.status(), `${p} ${step}`).toBe(404);
        expect(res.headers()["location"], `${p} ${step}`).toBeUndefined();
        expect(res.headers()["set-cookie"], `${p} ${step}`).toBeUndefined();
      }
    }
  });

  test("a configured provider's start sends the browser to the provider, with PKCE and a locked-down cookie", async ({
    request,
    baseURL,
  }) => {
    const providers = await configured(request);
    test.skip(providers.length === 0, "no provider is configured on this stack");
    for (const p of providers) {
      const res = await request.get(
        `/api/v1/auth/${p}/start?next=https://evil.example&redirect_uri=https://evil.example`,
        { maxRedirects: 0 },
      );
      expect(res.status()).toBe(302);
      const location = new URL(res.headers()["location"] ?? "");
      expect(location.protocol).toBe("https:");
      expect(location.hostname).toBe(AUTHORIZATION_HOST[p]);
      expect(location.searchParams.get("code_challenge_method")).toBe("S256");
      expect(location.searchParams.get("code_challenge")).toMatch(/^[A-Za-z0-9_-]{43}$/);
      expect(location.searchParams.get("state")).toMatch(/^[A-Za-z0-9_-]{43}$/);
      expect(location.searchParams.get("redirect_uri")).toBe(
        `${baseURL}/api/v1/auth/${p}/callback`,
      );
      expect(location.href).not.toMatch(/evil|secret/i);

      const cookie =
        res.headersArray().find((h) => h.name.toLowerCase() === "set-cookie")?.value ?? "";
      expect(cookie).toMatch(/^__Host-rm_oauth=[A-Za-z0-9_-]{43};/);
      for (const attribute of ["Path=/", "Secure", "HttpOnly", "SameSite=Lax", "Max-Age=600"]) {
        expect(cookie, attribute).toContain(attribute);
      }
      expect(cookie).not.toMatch(/Domain=/i);
    }
  });

  test("a callback with a bad state lands on /login with a plain message", async ({
    page,
    request,
  }) => {
    const providers = await configured(request);
    test.skip(providers.length === 0, "no provider is configured on this stack");
    for (const p of providers) {
      // Straight from the bridge: a 302 to a fixed path, clearing the attempt cookie.
      const raw = await request.get(
        `/api/v1/auth/${p}/callback?code=x&state=bogus&next=https://evil.example`,
        { maxRedirects: 0 },
      );
      expect(raw.status()).toBe(302);
      expect(raw.headers()["location"]).toBe("/login?error=invalid_state");
      const cookie =
        raw.headersArray().find((h) => h.name.toLowerCase() === "set-cookie")?.value ?? "";
      expect(cookie).toMatch(/^__Host-rm_oauth=;/);
      expect(cookie).toMatch(/Max-Age=0|Max-Age=-1|Expires=/i);

      await page.goto(`/api/v1/auth/${p}/callback?code=x&state=bogus`);
      await expect(page).toHaveURL(/\/login\?error=invalid_state$/);
      await expect(alerts(page)).toContainText(/expired or was already used/i);
    }
  });

  test("the callback passes on nothing but code, state and error", async ({ request }) => {
    const providers = await configured(request);
    test.skip(providers.length === 0, "no provider is configured on this stack");
    const p = providers[0]!;
    const huge = "a".repeat(5000);
    for (const query of [
      `?state=${huge}`,
      `?code=${huge}&state=x`,
      "?state=has%20space",
      "?state=caf%C3%A9",
    ]) {
      const res = await request.get(`/api/v1/auth/${p}/callback${query}`, { maxRedirects: 0 });
      expect(res.status(), query.slice(0, 30)).toBe(302);
      expect(res.headers()["location"]).toBe("/login?error=invalid_state");
    }
  });
});

test.describe("what the login page says about a failed sign-in", () => {
  const MESSAGES: Record<string, RegExp> = {
    access_denied: /cancelled/i,
    invalid_state: /expired or was already used/i,
    no_verified_email: /verified email/i,
    provider_conflict: /different account of that provider/i,
    identity_in_use: /already connected to another readmely account/i,
    unsupported_email: /could not use the email/i,
    provider_error: /did not answer as expected/i,
    account_unavailable: /cannot sign in right now/i,
  };

  for (const [code, message] of Object.entries(MESSAGES)) {
    test(`${code} shows a plain message and never the code`, async ({ page }) => {
      await page.goto(`/login?error=${code}`);
      const alert = alerts(page).filter({ hasText: message });
      await expect(alert).toBeVisible();
      await expect(alert).not.toContainText(code);
      await expect(page.getByLabel("Email address")).toBeVisible();
    });
  }

  test("an unknown code, or markup in it, shows nothing and injects nothing", async ({ page }) => {
    for (const code of [
      "made_up",
      encodeURIComponent("<img src=x onerror=alert(1)>"),
      "__proto__",
      "constructor",
    ]) {
      await page.goto(`/login?error=${code}`);
      await expect(page.getByLabel("Email address")).toBeVisible();
      await expect(alerts(page)).toHaveCount(0);
      await expect(page.locator("img[src='x']")).toHaveCount(0);
    }
  });
});

test.describe("connected accounts", () => {
  test("a signed-in user who lands on /login with an error is taken to /account with the reason", async ({
    page,
  }) => {
    await signIn(page, freshEmail("oautherr"));
    await page.goto("/login?error=identity_in_use");
    await expect(page).toHaveURL(/\/account\?error=identity_in_use$/);
    await expect(alerts(page).filter({ hasText: /already connected to another/i })).toBeVisible();
    // An unknown code is not carried over, and shows nothing.
    await page.goto("/login?error=made_up");
    await expect(page).toHaveURL(/\/account$/);
    await expect(alerts(page)).toHaveCount(0);
  });

  test("the account page offers Connect for each configured provider", async ({
    page,
    request,
  }) => {
    const providers = await configured(request);
    test.skip(providers.length === 0, "no provider is configured on this stack");
    await signIn(page, freshEmail("connect"));
    await expect(page.getByRole("heading", { name: "Connected accounts" })).toBeVisible();
    for (const p of providers) {
      await expect(
        page.getByRole("button", { name: new RegExp(`^connect ${p}$`, "i") }),
      ).toBeVisible();
    }
  });

  test("Connect leaves for the provider, and a failed return explains itself and spends the attempt", async ({
    page,
    request,
  }) => {
    const providers = await configured(request);
    test.skip(providers.length === 0, "no provider is configured on this stack");
    const p = providers[0]!;
    await signIn(page, freshEmail("connectflow"));

    // Stand in for the provider's page, and read what the browser was sent there with.
    let sent: URL | null = null;
    await page.route(`https://${AUTHORIZATION_HOST[p]}/**`, async (route) => {
      sent = new URL(route.request().url());
      await route.fulfill({ status: 200, contentType: "text/html", body: "<p>the provider</p>" });
    });
    await page.getByRole("button", { name: new RegExp(`^connect ${p}$`, "i") }).click();
    await expect(page.getByText("the provider")).toBeVisible();
    expect(sent).not.toBeNull();
    const state = sent!.searchParams.get("state") ?? "";
    expect(state).toMatch(/^[A-Za-z0-9_-]{43}$/);
    expect(sent!.searchParams.get("code_challenge_method")).toBe("S256");

    // The browser now holds the attempt cookie.
    const cookies = await page.context().cookies();
    expect(cookies.find((c) => c.name === "__Host-rm_oauth")).toMatchObject({
      httpOnly: true,
      secure: true,
      sameSite: "Lax",
    });

    // The provider sends the browser back. The state and cookie are right, the code is not one a
    // provider ever issued, so the exchange fails and the person is told, on the account page.
    await page.goto(`/api/v1/auth/${p}/callback?code=not-a-real-code&state=${state}`);
    await expect(page).toHaveURL(/\/account\?error=provider_error$/);
    await expect(alerts(page).filter({ hasText: /did not answer as expected/i })).toBeVisible();
    await expect(
      page.getByRole("button", { name: new RegExp(`^connect ${p}$`, "i") }),
    ).toBeVisible();

    // Nothing was connected, and the same return cannot be replayed.
    await page.goto(`/api/v1/auth/${p}/callback?code=not-a-real-code&state=${state}`);
    await expect(page).toHaveURL(/\/account\?error=invalid_state$/);
  });

  test("an account connected to a provider can be disconnected, after a confirmation", async ({
    page,
  }) => {
    const email = freshEmail("disconnect");
    await signIn(page, email);
    seedIdentity(email, "github", `e2e-${Date.now()}`);
    await page.reload();

    await expect(page.getByRole("heading", { name: "Connected accounts" })).toBeVisible();
    await expect(
      page.getByRole("list", { name: "Connected accounts" }).getByText(email.toLowerCase()),
    ).toBeVisible();
    await page.getByRole("button", { name: "Disconnect GitHub" }).click();

    // Asked first; backing out changes nothing.
    const confirm = page.getByRole("group", { name: /confirm disconnecting github/i });
    await expect(confirm).toContainText(/still sign in with an emailed code/i);
    await confirm.getByRole("button", { name: "Keep it" }).click();
    await expect(confirm).toHaveCount(0);
    expect(identityCount(email)).toBe(1);

    await page.getByRole("button", { name: "Disconnect GitHub" }).click();
    await page.getByRole("button", { name: /yes, disconnect github/i }).click();
    // Gone from the page (the whole card goes when no provider is configured either).
    await expect(page.getByRole("button", { name: "Disconnect GitHub" })).toHaveCount(0);
    expect(identityCount(email)).toBe(0);

    // Reloading shows the same, and email sign-in is untouched.
    await page.reload();
    await expect(page.getByText("Signed in as")).toBeVisible();
  });

  test("the API refuses to connect or disconnect without the page's CSRF token", async ({
    page,
  }) => {
    const email = freshEmail("csrfid");
    await signIn(page, email);
    seedIdentity(email, "google", `e2e-${Date.now()}`);
    const result = await page.evaluate(async () => {
      const call = async (method: string, url: string) =>
        (await fetch(url, { method, credentials: "same-origin" })).status;
      return {
        start: await call("POST", "/api/v1/me/identities/google/start"),
        remove: await call("DELETE", "/api/v1/me/identities/google"),
        list: await call("GET", "/api/v1/me/identities"),
      };
    });
    expect(result).toEqual({ start: 403, remove: 403, list: 200 });
    expect(identityCount(email)).toBe(1);
  });
});
