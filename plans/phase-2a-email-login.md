# Phase 2a · Sessions and passwordless email login

**Status:** Approved (2026-11-18) · **Last updated:** 2026-11-18

Phase 2 is split into three slices, each with its own plan, branch and sign-off: **2a** (this one) sessions and email-code login with account basics, **2b** Google and GitHub login with safe account linking, **2c** TOTP, recovery codes, account deletion and the settings UI.

Decisions already made: passwordless email codes (no passwords anywhere); opaque session cookie stored in Postgres; a `Mailer` interface with a log or file mailer in development; three slices.

## Goal

A person can enter their email, receive a one-time code, and be signed in. First sign-in creates the account (the code proves they own the address). They can see who they are, change their display name, see their active sessions, revoke any of them, and sign out. Everything is rate limited. No password exists to steal, stuff or reset.

## Out of scope

Google and GitHub login (2b). TOTP, recovery codes, step-up for sensitive actions, account deletion and the full settings UI (2c). Projects and GitHub import (Phase 3). Choosing and wiring a production email provider (MVP release; the interface is ready for it). Disposable-address blocking.

## Flow

1. The login page asks for an email and calls `POST /v1/auth/email/start`.
2. The API normalises the email, checks the rate limits, stores a **login challenge** (hashed code, expiry, attempt counter) and sends the code through the `Mailer`. It answers `202` **with the same body and similar timing whether or not the address has an account**, so the form cannot be used to discover who is registered.
3. The page asks for the 6-digit code and calls `POST /v1/auth/email/verify` with the email and code.
4. On success the API consumes the challenge (single use), creates the user if this is a first sign-in (marking `email_verified_at`), creates a **session**, and sets the cookie. A suspended account gets the same generic failure as a wrong code and no session.
5. Later requests carry the cookie. State-changing requests must also send the CSRF token (§Security). `POST /v1/auth/logout` deletes the session.

Sign-up and login are the same action. There is no separate sign-up form and no "email already registered" message.

## Data

One migration adds two tables. `users` already exists and keeps credentials out of it.

**`login_challenges`**: `id` (uuidv7), `email` (normalised, lower case), `code_hash` (HMAC-SHA-256 of the code, keyed with a server secret; a 6-digit code is too small to protect with a bare hash), `attempts` (int, default 0), `max_attempts` (5), `expires_at` (10 minutes), `consumed_at`, `created_at`, `requested_ip` (inet, for abuse review, kept 30 days). Constraints: attempts between 0 and max, email length ≤ 254. Index on `(email, created_at desc)` and on `expires_at` for cleanup. A new challenge for an email invalidates its older unconsumed ones (one live code per address).

**`sessions`**: `id` (uuidv7, the public handle used by "revoke this session"), `user_id` (FK, `ON DELETE CASCADE`, indexed), `token_hash` (SHA-256 of a 256-bit random token; unique; the token itself is never stored), `created_at`, `last_seen_at`, `expires_at` (absolute, 30 days), `idle_expires_at` (7 days, slides on use), `user_agent` (truncated to 200), `ip` (inet). A database `CHECK` ties `idle_expires_at <= expires_at`.

`readmely_app` may read and write both tables (default privileges). Cleanup of expired rows is one query run on a timer in the API process (hourly), tested.

The Phase 1 design listed `auth_credentials` and `email_verifications`; a passwordless design needs neither (`login_challenges` replaces the second). `totp_secrets`, `recovery_codes` and `oauth_identities` arrive with 2c and 2b.

## API (contracts-first, added to `contracts/core-public.yaml`)

| Method and path | Purpose | Auth |
|---|---|---|
| `POST /v1/auth/email/start` `{email}` | send a code | none; origin checked; rate limited |
| `POST /v1/auth/email/verify` `{email, code}` | sign in | none; origin checked; rate limited |
| `POST /v1/auth/logout` | end this session | session + CSRF |
| `GET /v1/me` | the user, plus a CSRF token for this session | session |
| `PATCH /v1/me` `{display_name}` | rename (1 to 80 chars, no control characters) | session + CSRF |
| `GET /v1/me/sessions` | list sessions (id, created, last seen, device, current flag) | session |
| `DELETE /v1/me/sessions/{id}` | revoke one session, own sessions only | session + CSRF |

Errors use the existing envelope. Codes: `invalid_request` (400), `unauthenticated` (401), `forbidden` (403, CSRF or origin), `invalid_code` (400, deliberately one answer for wrong, expired, used, unknown address and suspended), `rate_limited` (429 with `Retry-After`). Generated Go, Python and TypeScript types and the conformance tests pick the contract up as they do today.

## Security

Mapped to threats T6, T7 and T10 in [security.md](security.md).

- **Code guessing:** 6 digits from a CSPRNG, 10 minute life, 5 attempts per challenge, single use, a new request invalidates the old code, plus a per-IP limit on verify attempts. At 5 tries the chance of guessing a live code is 5 in 1,000,000.
- **Account enumeration:** `start` answers identically and in similar time for known and unknown addresses (the email is sent in the background, never awaited). `verify` has one failure message.
- **Mail bombing and cost abuse:** Redis limits: 1 code per address per 60 seconds, 5 per address per hour, 20 per IP per hour; verify 30 per IP per 15 minutes. Limits apply before any email is sent. The client IP comes from `X-Forwarded-For` only when the request arrives from a configured trusted proxy, otherwise from the socket.
- **Session theft and fixation:** 256-bit random token in `__Host-rm_session` (`Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`, no `Domain`); only its hash is stored; a new session id is minted on every login; logout and revoke delete the row, so they take effect on the next request; idle and absolute expiry are enforced in the query, not just the cookie.
- **CSRF (T7):** `SameSite=Lax`, an `Origin` check on every state-changing request against an allow-list (`WEB_ORIGIN`), `application/json` required, and a per-session CSRF token (HMAC of the session id) returned by `GET /v1/me` and sent as `X-CSRF-Token`. The web app calls the API through same-origin `/api/*` rewrites, so no CORS is enabled at all.
- **Secrets and logs:** the code is never logged in production. The log mailer and the file mailer refuse to start when `APP_ENV=production`, and startup fails clearly if no production mailer is configured. Emails, codes and tokens are redacted from request logs.
- **Open redirects and host header:** there is no `redirect` parameter; the post-login destination is a fixed route. Links in emails (none in 2a, the email carries only the code) would be built from configuration, never from request headers.
- **Email handling:** trimmed, lower-cased, ≤ 254 characters, no control characters, one `@`. Sub-address tricks (`+tag`) are kept as given, never merged.
- **Secrets configuration:** `AUTH_CODE_KEY` (≥ 32 bytes, for code HMACs and CSRF tokens) is required and validated like the existing secrets; `.env.example`, compose and CI get a placeholder.

## Web

`/login` (email step, then code step, with resend after the cooldown and clear errors), an authenticated `/account` page showing the user, an inline display-name editor, the session list with a revoke button, and sign out. Unauthenticated visits to `/account` redirect to `/login`; the check happens on the server by calling `/v1/me`, never by trusting a client flag. The CSP from Phase 0 is unchanged. Accessible form labels, focus handling, and error text read by screen readers.

## Scenarios (each becomes a test)

1. A new address signs in with the correct code and gets an account, a verified email and a session cookie with the right flags.
2. An existing address signs in and no duplicate user appears; upper-case and padded emails are the same account.
3. A wrong code is rejected; five wrong codes lock that challenge even if the sixth is correct.
4. An expired code, a used code and a superseded code are all rejected.
5. `start` returns the same status, body and (within a tolerance) time for a known and an unknown address.
6. Rate limits: the 2nd code within 60 s, the 6th in an hour, and per-IP limits return 429 with `Retry-After`; no email is sent for a limited request.
7. A suspended user cannot sign in and cannot tell why.
8. A session works until it idles out or reaches its absolute limit; both are enforced even if the cookie is still present.
9. Logout and "revoke" end a session immediately; one user cannot list or revoke another user's session (tenancy test, as in Phase 1).
10. A state-changing request without the CSRF token, with a wrong token, from a foreign `Origin`, or with a non-JSON body is refused.
11. Cookie attributes are exactly `__Host-` prefixed, `Secure`, `HttpOnly`, `SameSite=Lax`.
12. Concurrency: 20 simultaneous correct `verify` calls with one code create exactly one session, and 20 simultaneous `start` calls respect the limits.
13. The log and file mailers refuse `APP_ENV=production`; production without a mailer fails to start.
14. Cleanup removes expired challenges and sessions and nothing else.
15. Browser: full login, reload keeps the session, sign out, revoke another session, redirect when signed out.
16. Display name rules (length, control characters) and that the response never contains token or hash fields.

## Test plan

Unit tests for normalisation, code generation (uniform digits, leading zeros), HMAC compare in constant time, CSRF token derivation and the limiter. Integration tests against real Postgres and Redis for the challenge and session lifecycles, constraints, concurrency and tenancy. Handler tests through the generated contract types, and the conformance suite for the new routes. Security fixtures for the CSRF, origin, enumeration-timing and cookie cases. Playwright for scenario 15 with the file mailer. As in Phase 1, every guard is proven by **planting the fault and watching the test fail** (for example removing the attempt limit, the single-use check, the origin check, or the ownership filter). The conventions test already holds the new tables to the schema rules.

## Steps

Each step is one reviewed commit (or a few) on branch `feat/auth-email`.

1. `feat(contracts)`: the endpoints and schemas, regenerated types, conformance stubs.
2. `feat(db)`: migration for `login_challenges` and `sessions`, sqlc queries, schema tests.
3. `feat(core)`: `Mailer` (log, file), the Redis limiter, config (`AUTH_CODE_KEY`, `WEB_ORIGIN`, trusted proxies, `MAILER`), with tests.
4. `feat(core)`: the auth service and handlers: start, verify, logout, me, sessions, CSRF and origin middleware, cleanup timer.
5. `feat(web)`: `/login`, `/account`, the same-origin API rewrite, server-side session check.
6. `test` and `docs(plans)`: Playwright flow, security fixtures, docs, plan outcome.

## Decisions to confirm when approving

1. **Same-origin `/api/*` rewrite** through Next.js instead of CORS between two origins (recommended: no CORS surface, simple cookies).
2. **File mailer** for tests and e2e (writes each email as a JSON file to a directory; never allowed in production), in addition to the log mailer for day-to-day development.
3. **Session lifetimes:** 7 days idle, 30 days absolute. Say if you want shorter.
4. **Production email provider** is not chosen in 2a. The app will refuse to start in production without one, so it has to be picked before the MVP release.
