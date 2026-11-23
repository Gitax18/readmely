# Phase 2b · Google and GitHub sign-in, connected accounts, sign-in alerts

**Status:** Built, in review (2026-11-23) · **Last updated:** 2026-11-23

Phase 2 slices: 2a email codes and sessions (done), **2b** (this one) Google and GitHub sign-in with safe account linking, 2c TOTP, recovery codes, account deletion and the full settings UI.

Decisions already made (2026-11-19): link automatically when the provider-verified email matches an existing account; "Sign in with GitHub" uses its own small GitHub OAuth App (Phase 3 gets a separate GitHub App for import); `/account` gets connect and disconnect; provider buttons appear only when that provider is configured; sign-in alerts by email are included.

## Goal

A person can click "Continue with Google" or "Continue with GitHub" on `/login` and be signed in with the same kind of session as an emailed code. A first sign-in creates the account. If the provider-verified email belongs to an existing account, the provider is attached to it. A signed-in person can connect or disconnect providers on `/account`. Email codes always keep working, so nobody can lock themselves out. The owner is emailed when a provider is connected or disconnected and when a sign-in comes from a device they have not used recently.

## Out of scope

TOTP, recovery codes and step-up (2c). Account deletion (2c). Importing repositories and the Phase 3 GitHub App. Other providers. Using the provider's tokens for anything after sign-in (they are discarded). A production mailer (the alerts use the same `Mailer` as the codes).

## Flow

**Sign in (intent = login)**

1. `/login` shows a button per configured provider. It is a plain link to `GET /api/v1/auth/{provider}/start`.
2. The API makes a random `state`, a PKCE `code_verifier`, and a random binding value. It stores `{provider, intent, verifier, hash(binding)}` in Redis under `hash(state)` for 10 minutes, sets the binding in a short-lived cookie `__Host-rm_oauth` (`Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`), and answers `302` to the provider with `state`, `code_challenge` (S256), the redirect URI and the minimal scopes (`openid email profile` for Google, `read:user user:email` for GitHub).
3. The provider sends the browser back to `GET /api/v1/auth/{provider}/callback?code=…&state=…` (the redirect URI is `WEB_ORIGIN` + that path, through the same-origin bridge).
4. The API takes the state record **once** (atomic get-and-delete), checks the provider and the binding cookie, exchanges the code with the verifier at the provider's token endpoint, reads the profile, and forgets the provider's tokens.
5. Resolution, in order:
   1. The provider account is already linked: sign that user in. A suspended user gets the generic failure.
   2. No verified email from the provider: refuse (`no_verified_email`).
   3. An account with that email exists and does not yet have this provider: attach the provider, mark the email verified, sign in, and email the owner ("Google connected").
   4. An account with that email exists and already has a different account of this provider: refuse (`provider_conflict`). Nothing is merged.
   5. No account: create it (verified email, display name from the profile if it passes the display-name rules), attach the provider, sign in.
6. A new session is minted as in 2a and the browser is redirected to `/account`. Any failure redirects to `/login?error=<code>` with a fixed code; provider error text is never shown or logged.

**Connect (intent = link)**: from `/account`, `POST /api/v1/me/identities/{provider}/start` (session and CSRF token, like every state change) returns `{authorization_url}` and the page navigates there. The state record also holds the user id and session id. The callback attaches the provider only if the session in the cookie is still that session. A provider account already linked to **another** user is refused (`identity_in_use`); there is no merge.

**Disconnect**: `DELETE /api/v1/me/identities/{provider}` (session and CSRF). Always allowed: the email code is always available. Emails the owner.

The link start is a POST on purpose. A GET that begins linking could be triggered from another site and attach the attacker's Google account to the victim; with a POST and the CSRF token it cannot.

## Data

One migration, `oauth_identities`: `id` (uuidv7), `user_id` (FK, `ON DELETE CASCADE`, indexed), `provider` (`CHECK in ('google','github')`), `provider_user_id` (Google `sub`, GitHub numeric id; the stable key, never the email), `email` (lower case snapshot for display), `created_at`, `last_login_at`. Unique `(provider, provider_user_id)` and unique `(user_id, provider)`. `readmely_app` reads and writes; the AI role has nothing. No tokens are stored anywhere. The OAuth state lives in Redis, not Postgres.

## API (contracts-first, added to `contracts/core-public.yaml`)

| Method and path | Purpose | Auth |
|---|---|---|
| `GET /v1/auth/providers` | which providers are configured (`["google","github"]`) | none |
| `GET /v1/auth/{provider}/start` | begin sign-in; 302 to the provider | none; rate limited |
| `GET /v1/auth/{provider}/callback` | finish sign-in; 302 to `/account` or `/login?error=…` | state and binding cookie; rate limited |
| `GET /v1/me/identities` | connected accounts (provider, email, connected on) | session |
| `POST /v1/me/identities/{provider}/start` | begin connecting; returns the authorization URL | session + CSRF |
| `DELETE /v1/me/identities/{provider}` | disconnect | session + CSRF |

`{provider}` is an enum of `google` and `github`; anything else is 404. Error codes on the login redirect: `access_denied`, `invalid_state`, `no_verified_email`, `provider_conflict`, `identity_in_use`, `unsupported_email`, `provider_error`, `account_unavailable` (suspended, deliberately vague). The conformance test is extended: redirects document `Location` and `Set-Cookie`, signed-in operations document 401, state changes require the CSRF header, no response mentions a token.

## Security

Mapped to T6 (OAuth CSRF, account-linking takeover) and T7 in [security.md](security.md).

- **OAuth CSRF and login CSRF:** `state` is 256 random bits, single use, 10 minutes, bound to the browser by the `__Host-rm_oauth` cookie (only its hash is in Redis). A callback without the cookie, with another browser's cookie, with a used or unknown state, or on the wrong provider path is refused.
- **Code interception:** PKCE (S256) on both providers; the verifier never leaves the server.
- **Linking takeover:** identities are keyed by the provider's stable id. Email-based linking happens only when the provider itself says the email is verified (Google `email_verified`; GitHub the **primary and verified** address from `/user/emails`). Unverified, missing, non-ASCII or malformed emails are refused. Two provider accounts never merge two users. One account can hold one identity per provider.
- **Connect CSRF:** link start is a POST with the session CSRF token and the Origin check; the state record carries the user and session, and the callback requires the same session.
- **No open redirect:** the destinations after a callback are the fixed paths `/account` and `/login?error=<code>`. The only URL the API ever redirects to is the provider's, built from constants. The web bridge forwards a `Location` only if it is a same-site path or starts with a provider's authorization origin.
- **Provider calls:** server side only, fixed hosts (no SSRF), TLS, 5 second timeouts, no redirects followed, bodies capped at 64 KiB, strict JSON decoding of only the fields used. Access tokens and codes are never stored or logged; the client secret comes from the environment and is never printed.
- **Sessions:** the callback mints a new session id exactly as 2a does; the cookie is set on the 302.
- **Alerts:** emailed through the 2a bounded queue; capped per user per hour so they cannot become a mail-bombing channel; they contain the provider name, the device label and the time, never a link that signs anyone in.
- **Enumeration:** callback outcomes reveal only the fixed error codes above; "email belongs to another account" is never stated.
- **Rate limits:** start 30 and callback 30 per client address per 15 minutes (the 2a limiter and client-address resolver).

## Web

`/login` gains provider buttons (only configured ones, from `/v1/auth/providers`, rendered on the server) and maps `?error=` codes to plain messages. `/account` gains "Connected accounts": each provider with its email and Connect or Disconnect. Disconnecting asks for confirmation. The bridge allow-list grows by the six paths; for the callback it forwards only `code`, `state`, `error` (bounded length) as the query, passes a 302 with its `Set-Cookie` headers, and applies the `Location` rule above.

## Configuration

`GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`: each pair optional, both or neither (a half pair is a startup error, naming the variable only). With no pair the provider is simply off. `.env.example`, Compose and the README get them. Redirect URIs are derived from `WEB_ORIGIN`.

**What you need to do to try it for real (not needed for tests or CI):**
- Google Cloud Console → Credentials → OAuth client (Web application), redirect URI `http://127.0.0.1:3000/api/v1/auth/google/callback`.
- GitHub → Settings → Developer settings → OAuth Apps, callback URL `http://127.0.0.1:3000/api/v1/auth/github/callback`.

## Scenarios (each becomes a test)

Go integration tests run the real handlers against a fake provider (an `httptest` server standing in for the token, profile and email endpoints) with real Postgres and Redis:

1. New Google user: account created, email verified, identity stored, session cookie set, redirected to `/account`. Same for GitHub.
2. Returning user: same identity signs in, no new user, `last_login_at` moves.
3. Existing email account, provider-verified email: the provider is attached to that user, no duplicate user, owner emailed.
4. Provider says the email is unverified, or GitHub has no verified primary: refused, no user, no identity.
5. Email matches an account that already has a different account of this provider: `provider_conflict`, nothing attached.
6. Missing, reused, expired or foreign state; missing or wrong binding cookie; wrong provider path; provider `error=access_denied`: each refused with the right code and no session.
7. PKCE: the token request carries the verifier that matches the challenge sent at start.
8. Provider token or profile endpoint failing, slow, redirecting, oversized or malformed: `provider_error`, no session, nothing logged but the status.
9. Suspended user via a linked identity: `account_unavailable`, no session.
10. Connect: needs a session and the CSRF token; the callback with a different session is refused; a provider account linked to another user gives `identity_in_use`; tenancy: one user cannot list or disconnect another's identities.
11. Disconnect works, takes effect at once, the user can still sign in by email; an unknown provider is 404.
12. Alerts: connect, disconnect and a sign-in from an unknown device send one email each; a known device sends none; the per-hour cap holds; the queue-full path stays invisible.
13. Concurrency: 20 simultaneous callbacks with one state produce exactly one session; two simultaneous first sign-ins for one provider account create one user and one identity.
14. Configuration: a half pair fails startup naming the variable; no pair hides the provider; secrets never appear in the error or log.
15. Provider emails that are non-ASCII, empty or malformed are refused; case differences match the same account.
16. Redirect rules: the API's `Location` is the provider's authorization URL or one of the fixed paths; nothing taken from the request ever reaches it.

Browser tests (Chromium, real stack): `/login` shows no provider buttons when none is configured (the CI default); the start route answers 404 for an unconfigured provider and for an unknown one; a callback with a bad state lands on `/login` with the message; the bridge forwards nothing but the allowed query. The full provider round trip is covered by scenarios 1 to 16, because the browser tests cannot reach a real provider and I will not add a development back door that points the API at a fake one.

## Test plan

Unit tests for state and PKCE generation, authorization URL building, profile decoding and email rules (table-driven). Integration tests for the scenarios above. Handler tests through the generated contract types and the conformance suite. As in 2a, every guard is proven by **planting the fault and watching the test fail** (for example reading the email without the verified flag, skipping the binding-cookie check, reusing a state, linking across users, attaching without the session check, dropping PKCE, following a redirect from the provider, forwarding any `Location`).

## Steps

Each step is one reviewed commit (or a few) on branch `feat/auth-oauth`.

1. `feat(contracts)`: the six operations and schemas, regenerated types, conformance rules.
2. `feat(db)`: `oauth_identities`, queries, schema tests.
3. `feat(core)`: provider clients (Google, GitHub), state store with PKCE, configuration, with tests against the fake provider.
4. `feat(core)`: the sign-in and connect handlers, linking rules, alerts, cleanup of nothing new.
5. `feat(web)`: provider buttons, error messages, "Connected accounts", bridge changes.
6. `test` and `docs(plans)`: browser tests, security fixtures, README, plan outcome.

## Decisions to confirm when approving

1. **Dependency:** `golang.org/x/oauth2` for the authorization-code and PKCE plumbing, or about 150 lines of our own against the two providers. Recommended: our own, because the flow is small, the providers are fixed, and it keeps every security-relevant step visible and testable; the library would still need the same checks around it.
2. **Google identity check:** read the profile from Google's `userinfo` endpoint with the access token received directly from the token endpoint over TLS, rather than parsing and verifying the ID token (no JWT code, no key rotation to follow).
3. **Alert rule for new devices:** email when the account existed before this sign-in and none of its live sessions has the same device label (browser and system). This applies to every sign-in method, email codes included. A person returning after all sessions expired gets one notice. Say if you want it narrower (provider sign-ins only) or off.
4. **Alert cap:** at most 5 alert emails per user per hour; extra alerts are dropped and counted, not queued.
5. **Disconnect confirmation** is a browser confirm step only (no email code), because the alert goes out and email sign-in remains. Step-up for sensitive actions arrives with 2c.

## Outcome (2026-11-23)

Steps 1 to 6 were built on `feat/auth-oauth` with you in the loop. This section records what differs from the plan above and the small decisions the plan did not settle, so any of them can be reversed by saying so.

### Decisions taken during the build (reverse any of them by saying so)

1. **A connect also needs a verified email from the provider**, although you are already signed in. The identity stores that address for display, and one rule for both flows is simpler to reason about. The address may differ from your account's; connecting is your own choice.
2. **A returning sign-in updates the identity's stored email** to what the provider reports now. The account's own email never changes, and the user is found by the provider's id, so a provider changing someone's address moves nobody.
3. **A name from the provider is used only for a new account, and only if it passes the display-name rules**; an existing name is never overwritten.
4. **Unexpected failures inside the callback (database, Redis) redirect to `/login?error=provider_error`** instead of showing an error page, so a navigation never ends on raw JSON. They are logged without request content.
5. **The callback answers 404 for a provider that is not configured**, so "a callback with a bad state lands on /login" is only observable for a configured provider. CI therefore starts the stack with a throwaway Google client (random values, no real client behind them) and the browser tests derive what they expect from `GET /v1/auth/providers`; they also pass with no provider, and with real ones.
6. **The bridge turns any redirect it does not recognise into a 502**: a 3xx from any route other than the two provider routes, a status other than 302, or a `Location` that is not a same-site path or a provider authorization URL. The browser client independently refuses to navigate to anything but a provider authorization URL.
7. **A signed-in person whose connect fails lands on `/login?error=`**, as for sign-in; because `/login` sends signed-in visitors on, the code is passed to `/account?error=` so the reason is not lost. Unknown codes show nothing.
8. **A connected provider stays listed on `/account` even when its credentials are later removed**, so it can still be disconnected.
9. **Google's account chooser is always shown** (`prompt=select_account`), so a person with several Google accounts picks one rather than being signed in with whichever is active.
10. **A brand new account gets no alert** (there is no usual device to compare with); an existing account signing in by email code from an unfamiliar device does (decision 3 of the plan, as approved).
11. **`GET /v1/auth/email/start` now answers 404, not 405**, because `email` matches the `{provider}` route and is not a provider. A 2a test was changed to check a real POST-only route and to pin this behaviour.
12. **Cache and referrer headers on the redirects come from the router's global middleware**, which already sets `no-store` and `no-referrer` on every response; tests assert the result.

### Bugs found by the new tests and fixed on this branch

- **The rate limiter could refuse a whole burst, the first request included** (2a code). Each rule was counted in its own Redis call, so concurrent requests could raise the slower counters before the first request reached them. A stress test of 150 bursts of 20 failed on the old code; all of a request's rules are now counted in one script. It showed up as a rare failure of a 2a test once the suite got bigger.
- **Two overlapping first sign-ins for the same provider account could report `provider_conflict`** to the loser. It now compares provider account ids and runs again. Random concurrency almost never hit the window, so a test seam makes the overlap certain; the test fails on the old code.

### Differences from the plan text

- The conformance rules treat the provider routes as their own kind (GET, 302 with `Location` and `Set-Cookie`, 404 and 429 documented); the `planned` list is empty again.
- The web session check, CSRF token and Origin rules are unchanged; the connect start and disconnect go through them like every other state change.

### Known limits (accepted for 2b)

- The provider round trip against a real provider has not been exercised by an automated test, by design (no back door). Try it once with real credentials (README, "Sign in with Google or GitHub").
- Per-address limits can still be used to delay someone's email sign-in (2a); a provider sign-in does not share those limits.
- Alerts are capped at five per user per hour; extra ones are dropped and logged, not queued.
- A person who disconnects every provider and loses access to their email has no other way in until 2c adds recovery.

### Evidence

- Go: 56 planted faults, each caught by the intended test (database 7, provider clients and state store 20, sign-in and linking 29: unverified email, wrong session, cross-user attach, second account per provider, suspended user, both rate limits, alert rules and cap, cookie attributes, redirects, secrets in logs, race retry, and more). Every response in the new Go tests is also validated against `core-public.yaml`.
- Web: 34 planted faults, each caught (bridge paths, callback query, `Location` rules, cookies on redirects, authorization URL checks, CSRF on connect and disconnect, confirmation step, error-code handling, provider list handling).
- Tests: 335 web unit tests (97.6% line coverage), 27 browser tests against the real stack (19 new), the Go integration suite for `auth`, `oauth`, `ratelimit`, `db/store` and `config`, and the conformance suite.
- Scenarios 1 to 16 are each covered (14 by the configuration tests, 16 by the redirect tests and the bridge tests).
