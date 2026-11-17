# Phase 0 · Foundations

**Status:** Approved (2026-11-15) · **Last updated:** 2026-11-15

## Goal

An empty but fully wired project. One command starts everything locally, every service answers a health check, and CI runs lint, tests, and security scans on every push. No product features.

## Out of scope

- Any database tables beyond what migrations tooling needs (Phase 1).
- Auth, users, projects (Phase 2 onwards).
- The ELK layout sidecar (diagram phase).
- Production deployment (MVP release).

## Toolchain

| Area | Choice |
|---|---|
| Go | 1.27 · `chi` router · `pgx` + `sqlc` · `goose` migrations · `slog` logging · `golangci-lint` |
| Python | 3.13, managed by `uv` (the system Python is 3.9 and is not used) · FastAPI · Pydantic · `ruff` · `mypy` · `pytest` |
| Frontend | Node 24 · `pnpm` · Next.js App Router · TypeScript (strict) · Tailwind · shadcn/ui · ESLint · Prettier · Vitest · Playwright |
| Contracts | OpenAPI 3.1 + JSON Schema · `oapi-codegen` (Go) · `datamodel-code-generator` (Python) · `openapi-typescript` (TS) |
| Local stack | Docker Compose: Postgres 17 with pgvector, Redis, RustFS (S3-compatible object storage) |
| Task runner | Root `Makefile` |
| Hooks | `lefthook` (pre-commit lint and format, commit-message check) |
| CI | GitHub Actions |

Go 1.27 and Node 24 are what is installed on the development machine, so they replace the 1.25 and 22 mentioned in discussion.

## Repository layout after this phase

```
apps/web/                 Next.js shell
services/core/            go.mod, cmd/{api,ingest,render}, internal/{config,httpx,logging,health}
services/ai/              pyproject.toml, app/ (FastAPI), worker entry point, tests/
contracts/                openapi/internal.yaml, openapi/public.yaml, schemas/
infra/                    docker-compose.yml, docker/ (Dockerfiles)
plans/
testdata/
Makefile  lefthook.yml  .env.example  .github/workflows/
```

## Steps

Each step is one branch, reviewed before the next begins.

### Step 1 · Skeleton and local data stores
- Folder layout, root `Makefile`, `.env.example`, `.editorconfig`.
- `infra/docker-compose.yml` with Postgres (pgvector image), Redis, and RustFS, each with a health check and a named volume.
- Data stores bind to `127.0.0.1` only, with credentials read from `.env`.
- **Verify:** `make up` starts all three and reports healthy; `make down` stops them; `CREATE EXTENSION vector` succeeds.

### Step 2 · Go core service
- One module, three binaries. `ingest` and `render` only start, log, and exit cleanly on a signal.
- `api`: config from environment with validation, structured JSON logs with a request ID, panic recovery, timeouts, graceful shutdown.
- `GET /healthz` (process is up) and `GET /readyz` (Postgres and Redis reachable).
- **Tests:** config validation (missing and malformed values), health handlers, readiness when a dependency is down, shutdown.
- **Added at review:** the API is containerised in this step (`infra/docker/core.Dockerfile`, distroless, non-root, read-only filesystem, all capabilities dropped; `api healthcheck` subcommand for the container probe). `make up-app` runs it in a container, `make run-api` runs it on the host bound to `127.0.0.1`. Also `make fmt`, `lint`, `test`, `test-integration`, `docker-build`.
- **Found by testing:** `http.Server.Shutdown` waits up to 5 s for connections that never sent a request, so shutdown now counts in-flight requests and only reports an error if real requests were cut off. A readiness probe that hangs can no longer mark a healthy dependency as down.

### Step 3 · Python AI service (revised after the [2026-11-17 audit](audit-2026-11-17.md))

**Scope:** Python only, in `services/ai`, plus its Dockerfile and compose entry. No LLM calls, no job processing, no Go changes.

**Layout**
```
services/ai/
  pyproject.toml, uv.lock     Python 3.13; every dependency pinned by the lockfile
  app/main.py                 app factory: create_app(settings, db) for easy testing
  app/config.py               Pydantic Settings, validated at start
  app/logging.py              JSON logs, stdlib only
  app/security.py             request ID, service-token check, body-size limit, generic errors
  app/health.py               /healthz, /readyz
  app/db.py                   lazily opened psycopg pool (the service starts even if Postgres is down)
  app/worker.py               worker entry point: start, log, stop on SIGTERM
  tests/                      unit tests + integration tests behind a marker
```

**Behaviour (identical contract to the Go service)**
- `GET /healthz` is unauthenticated and never touches a dependency. It's used by the container probe.
- `GET /readyz` **requires the service token** (audit A1). It pings Postgres with a 2 s limit and returns `{"status": "ready"|"unavailable", "checks": {"postgres": "ok"|"down"}}` with 200 or 503. The reason goes to the log only. The Go API will call it in Step 4.
- **Service token:** every route except `/healthz` requires `Authorization: Bearer <AI_SERVICE_TOKEN>`, compared in constant time. Token minimum length 32; the service refuses to start without one. This is the first piece of threat T12 and the pattern every later internal endpoint inherits.
- Error body `{"error": "<code>", "request_id": "..."}` for 401, 404, 405, 413, 422 and 500. No stack traces or validation details that echo input.
- Request ID: same rule as Go (`^[A-Za-z0-9._-]{8,64}$`, otherwise generated), returned in `X-Request-ID`.
- Request bodies capped at 1 MiB (checked on `Content-Length` and while streaming).
- `/docs`, `/redoc` and `/openapi.json` disabled. Server header removed.
- Settings: `APP_ENV`, `LOG_LEVEL`, `HTTP_PORT`, `DATABASE_URL`, `AI_SERVICE_TOKEN`, `SHUTDOWN_TIMEOUT`. All problems reported at once; values never printed. Credentials must be URL-safe (audit A2).
- Graceful shutdown: in-flight requests finish (Uvicorn graceful timeout), the pool closes, exit code 0.
- Worker: separate process, `python -m app.worker`; starts, logs, exits 0 on SIGTERM or SIGINT, 2 on bad config.

**Container and compose**
- `infra/docker/ai.Dockerfile`: multi-stage build with `uv`, `python:3.13-slim` runtime, non-root user, no build tools in the final image, bytecode precompiled. Healthcheck through a tiny stdlib Python probe (no curl).
- Compose service `ai` in the `app` profile: **no published port** (internal network only), read-only filesystem with a small `/tmp` tmpfs, all capabilities dropped, `no-new-privileges`.
- `.env.example` gains `AI_SERVICE_TOKEN`, generated with `openssl rand -hex 32`.

**Tooling**
- FastAPI, Uvicorn, Pydantic Settings, `psycopg[binary]` 3 with `psycopg-pool`.
- `ruff` (lint, format, and the `S` security rules), `mypy --strict`, `pytest` with `pytest-asyncio` and `pytest-cov` (**fails under 90% coverage**), `pip-audit`.
- `make lint`, `make test`, `make fmt` run Go and Python; `make test-integration` covers both; new `make run-ai`.

**Scenarios to test**
- Settings: each variable missing or malformed; several problems reported together; a planted password never appears in the error; token shorter than 32 characters rejected.
- `/readyz`: no token → 401, wrong token → 401, right token → 200; Postgres down → 503; Postgres hung → 503 within the timeout; failure reason absent from the response.
- `/healthz` works without a token and while Postgres is down.
- Service starts and serves `/healthz` while Postgres is unreachable.
- Request ID: valid client value kept, unsafe values replaced.
- Errors: unknown route 404, wrong method 405, oversized body 413, handler exception 500, each in the standard body with no internal details.
- Shutdown: SIGTERM during an in-flight request lets it finish; exit code 0.
- Worker: starts, stops on signal with exit 0; bad config exits 2.
- Integration (real Postgres): ready; wrong password → down.
- Container: image builds, runs as non-root, port not reachable from the host, healthy; Postgres stopped → `/readyz` 503 (checked from inside the compose network).
- Response shapes match the Go service exactly (audit B4).

**Outcome (2026-11-17):** done. 82 unit tests plus 2 integration tests, 97% coverage measured including subprocesses, ruff and `mypy --strict` clean, every scenario above passing, image 54 MB, non-root.
- **Found by testing:** Uvicorn re-raises SIGTERM after a graceful shutdown, so the process died with -15 (Docker 143) instead of exiting 0; `main()` now installs its own stop handlers, which also covers a signal during startup. Uvicorn exits with its own code 3 when it cannot bind; mapped to our documented 1. A request that crashed was missing from the access log; logging moved into `finally`. Error responses could carry the request ID and security headers twice; now added only when absent.
- **Added at review:** `make audit` (govulncheck and pip-audit). Its first run found two Go standard-library vulnerabilities; recorded as audit item A9, fixed first on `fix/audit-items`.

### Step 4 · Next.js shell
**Status:** Done 2026-11-18 (agreed 2026-11-18: server-side fetch, detailed status, pnpm + Vitest + Playwright, containerised).

**Goal:** a secure Next.js app that renders one status page for the API and the AI service, with the whole path (browser → Next.js server → Go → Python) working and tested.

**Data flow.** The browser only talks to Next.js. The page is a server component that fetches `GET /api/v1/status` from the Go API over the internal network (`API_INTERNAL_URL`, server-only, never `NEXT_PUBLIC_*`). The Go API asks the AI service for its `/readyz` with the service token. No CORS, no token or internal address in the browser.

**Go: `GET /api/v1/status`** (served on the **internal listener only**, like `/readyz`, so the detail never appears on the public port)
- New config: `AI_SERVICE_URL` (default `http://ai:8000`) and `AI_SERVICE_TOKEN` (optional, 32+ chars if set; empty means the AI check reports `unconfigured`). Values never printed.
- Always 200 when Go itself is running; it is a report. Body:
  `{"status": "operational|degraded|unavailable", "services": {"api": {"status": "ok|down", "checks": {"postgres": "ok|down", "redis": "ok|down"}}, "ai": {"status": "ok|down|unreachable|unconfigured", "checks": {"postgres": "ok|down"}}}}`
- Overall: `operational` if everything is ok; `unavailable` if the API's own Postgres is down; otherwise `degraded`.
- AI call: 2 s timeout, Bearer token, **redirects not followed** (the token must never go to another host), response read through a 64 KiB limit, body parsed against a strict shape, unknown values treated as `down`. Failure reasons go to the log only.
- `api` checks reuse the existing readiness probes; the AI call runs in parallel with them so one hung part can't stall the report.

**Web app (`apps/web`)**
- Next.js 16, React 19, Tailwind 4, shadcn/ui (Card and Badge only), strict TypeScript, ESLint, Prettier. `pnpm` with exact versions, committed lockfile, install scripts blocked except an allowlist, `pnpm audit` in `make audit`.
- `/` status page, rendered per request (`no-store`, 3 s fetch timeout). Four states: operational, degraded, unavailable, **API unreachable**. Status is shown as text and icon, never colour alone.
- The API response is untrusted: validated with Zod; anything that doesn't match is shown as "invalid response from API". Values are rendered as text only.
- Detail level: `STATUS_DETAIL=full|coarse`. `full` (default in development) shows each dependency; `coarse` shows only overall and per-service state. Redaction happens on the server, so hidden data is never sent to the browser. **Proposal: production defaults to `coarse`**, because a public page naming internal dependencies helps attackers; you can override that when we deploy.
- `GET /healthz` route for the container healthcheck.
- Environment validated at startup; errors name the variable and never the value.

**Security headers** (set per request in the Next.js proxy/middleware): CSP with a per-request nonce (`default-src 'self'`, `script-src 'self' 'nonce-…' 'strict-dynamic'`, `object-src 'none'`, `base-uri 'self'`, `frame-ancestors 'none'`, `form-action 'self'`; no `unsafe-eval` or `unsafe-inline` in production), `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, `X-Frame-Options: DENY`, a locked-down `Permissions-Policy`, and `X-Powered-By` removed. HSTS is added at deploy time behind TLS.

**Container and compose**
- `infra/docker/web.Dockerfile`: multi-stage pnpm build, Next.js `standalone` output, pinned Node 24 slim/alpine image, non-root user, no package manager in the final image, healthcheck through Node.
- Compose service `web` in the `app` profile: published on `127.0.0.1:${WEB_PORT:-3000}` only, read-only filesystem with a `/tmp` tmpfs, capabilities dropped, `no-new-privileges`, `API_INTERNAL_URL=http://api:8081`.
- `.env.example` gains `WEB_PORT`, `AI_SERVICE_URL`. Go service in compose gets `AI_SERVICE_URL` and `AI_SERVICE_TOKEN`.
- Makefile: `make run-web` (dev server bound to `127.0.0.1`); `lint`, `test`, `fmt`, `test-integration`, `audit` and `docker-build` include the web app; `verify` also checks the web port is local-only.

**Tests**
- Go (`/api/v1/status`, fake AI server): all ok; AI 503 with detail; AI hangs (timeout, API still answers within 3 s); AI returns 401 (reason absent from the response); AI redirects (second server receives no request, so no token leak); oversized and malformed AI bodies; unknown status values; token not configured; AI down gives `degraded`; API Postgres down gives `unavailable`; token never in logs or response; config validation for the two new variables; not served on the public router.
- Web unit (Vitest + Testing Library): each of the four states; `full` vs `coarse` output (hidden detail absent from the HTML); malformed and hostile payloads (script text rendered as text, wrong types, extra fields); fetch timeout and network error; environment validation.
- Playwright (against a small fake API): page loads and shows operational; shows degraded; shows API unreachable when the fake is stopped; response headers present, CSP nonce changes per request, no inline script without a nonce; `/healthz`; keyboard and semantics check (landmarks, heading, status text).
- Container: image builds, runs as non-root, read-only filesystem works, healthy; page returns 200 through the real stack (web → Go → Python); stop the AI container and the page shows degraded; stop Go and the page shows API unreachable; web port bound to `127.0.0.1` only.
- Whole-repo: `make lint test test-integration audit verify` all green.

**Commits (one branch, `feat/web-shell`)**
1. `feat(core): add internal GET /api/v1/status` (+ config, tests)
2. `feat(web): scaffold Next.js app with Tailwind and shadcn/ui`
3. `feat(web): status page with validated API client`
4. `security(web): add CSP and security headers`
5. `build(infra): containerise the web app and add make run-web`
6. `test(web): Playwright page and header tests`
7. `docs(plans): record Step 4 outcome`

**Outcome (2026-11-18):** implemented as planned on `feat/web-shell` (7 commits). 60 Go tests (15 new, `internal/status` at 95.5% coverage), 82 Python tests (unchanged), 69 web unit tests (100% of lines), 16 Playwright tests, lint and `make audit` clean in all three languages. Web image 68 MB, non-root, read-only, distroless. Checked live in containers: full path operational; AI stopped gives degraded; API stopped gives "API unreachable" while the web container stays healthy; Postgres stopped gives unavailable; everything recovers; `/api/v1/status` returns 404 on the public port and the internal port is not reachable from the host; the AI token appears in no log.
- **Found by testing or tooling:**
  - pnpm's 7-day release-age rule rejected the freshly resolved lockfile (64 packages published within a week). Re-resolved so every dependency is at least 7 days old.
  - `typescript-eslint` does not support TypeScript 7, so TypeScript is pinned to 6.0.3. ESLint is pinned to 9.39.5 because the Next.js lint plugins do not yet support ESLint 10 (npm marks 9.x as end of life; revisit when the plugins catch up).
  - A Go test deadlocked because cleanups run in reverse order: the fake server waited on a handler that was only released afterwards. Fixed the order.
  - A browser test showed that an inline script created through the DOM runs under `'strict-dynamic'`. That is by design (trust passes from nonce'd scripts); the real attack, an injected `<script>` in the HTML, is blocked and now tested that way.
  - Zod drops a `__proto__` key instead of rejecting it; verified no pollution and tested it.
- **Decision recorded:** production defaults `STATUS_DETAIL` to `coarse`. Change before deploy if the detailed public view is wanted.
- **Open for later (deploy step):** HSTS and `upgrade-insecure-requests` once TLS terminates in front of the app.

### Step 5 · Contracts and type generation
**Status:** Approved 2026-11-18 (agreed: one spec per server, conformance tests on both servers, keep Zod and assert it matches the generated types).

**Goal:** the API shapes are written down once, in OpenAPI files, and nothing can drift from them: types in all three languages are generated from the specs, and tests prove the real servers match the specs.

**Specs (`contracts/`, OpenAPI 3.0.3).** 3.0.3 rather than 3.1 because the Go validator (`kin-openapi`) has no confirmed 3.1 support and `oapi-codegen` only has an initial one; every tool we use handles 3.0.3 fully. Move to 3.1 once the tools catch up.
- `core-public.yaml`: the Go API's public listener (`GET /healthz`).
- `core-internal.yaml`: the Go API's internal listener (`GET /healthz`, `GET /readyz`, `GET /api/v1/status`). Documented as reachable on the internal network only.
- `ai.yaml`: the AI service (`GET /healthz`, `GET /readyz` with bearer auth).
- Every object has `additionalProperties: false`; status values are enums; check names have the same pattern and 8-entry cap the code enforces. Each response documents `X-Request-ID`; errors share one `Error` schema (`error`, `request_id`); 401, 405 and 503 are documented where the server can send them. Each response has examples (every state of `/api/v1/status`, ready and unavailable).
- Files are self-contained. The few shared schemas (`Error`, liveness, readiness) are repeated, and `make contracts-check` fails if the copies differ.

**Generation (`make gen`, output committed, marked `linguist-generated`).**
- Go: `oapi-codegen` v2.8.2 (pinned as a `tool` in `go.mod`), models only, into `internal/contract/{corepublic,coreinternal,ai}`.
- Python: `datamodel-code-generator` 0.97 (dev dependency), Pydantic v2 models for `ai.yaml` into `app/contract/`.
- TypeScript: `openapi-typescript` 7.13 (pinned dev dependency), `core-internal.d.ts` into `apps/web/src/contract/`, plus the spec's examples extracted to JSON for tests.
- Output is deterministic (no timestamps). `make gen-check` regenerates and fails if `git` shows any difference or untracked generated file. It is wired into CI in Step 6.
- Generated files are excluded from lint, formatting and coverage; mypy still checks the Python models.
- New tooling is dev-only and never reaches an image. `make audit` now also scans the Python dev group, so the generators are covered.

**Code uses the generated types (so a shape change breaks the build, not just a test).**
- Go: `health` (liveness, readiness), `status` (`Report`, `Service`, and the AI response it parses) and `httpx` (error body) switch to the generated types.
- Python: `health.py` and `security.py` build their responses from the generated models.
- Web: the hand-written Zod schema stays (strict runtime validation, already tested). A compile-time check requires it to match the generated type in both directions; a fixture with a deliberately drifted schema and `@ts-expect-error` proves the check really fails.

**Conformance tests (the spec cannot lie).**
- Go (`kin-openapi`, test-only): every response of both routers is validated against its spec: all `/api/v1/status` states, `/readyz` 200 and 503, `/healthz`, wrong-method 405, the unknown-route 404 body. A route-parity test walks the chi routers: every registered route is in the spec and every spec path is registered.
- Python (`openapi-core`, test-only): the same for the AI service: `/healthz`, `/readyz` 200, 503 and 401, 405, oversized 413, plus route parity against the FastAPI app.
- Web: every example in `core-internal.yaml` must pass the Zod schema, so the runtime validator accepts everything the spec documents.
- Parity checkers are themselves tested against a planted extra route and a planted missing route.

**Commits (branch `feat/contracts`)**
1. `feat(contracts): add OpenAPI specs for core and AI` (specs, `make contracts-check`)
2. `build(contracts): generate Go, Python and TypeScript types` (`make gen`, `make gen-check`, tool pins, excludes)
3. `refactor(core): use generated contract types` (+ Go conformance and parity tests)
4. `refactor(ai): use generated contract types` (+ Python conformance and parity tests)
5. `refactor(web): check the Zod schema against generated types` (+ examples test, drift fixture)
6. `docs(plans): record Step 5 outcome`

**Tests / verify (each planted, seen failing, then reverted)**
- Edit a spec without running `make gen`: `make gen-check` fails. Hand-edit a generated file: fails. Clean tree: passes and is stable over repeated runs.
- A deliberately invalid spec is rejected by `make contracts-check`; so are two shared schemas that differ.
- Add a route in Go (or FastAPI) without a spec entry, and delete one spec path: parity tests fail.
- Change a handler's response field name or type: conformance test and compile both fail.
- Change the Zod schema (add, drop or retype a field): `tsc` fails; change a spec example into something Zod rejects: the web test fails.
- Whole repo: `make lint test test-e2e test-integration audit gen-check` all green; all three images still build.

**Out of scope:** generated clients or server stubs (types only for now), contracts for endpoints that don't exist yet (each later feature adds its own, in its own plan), and the CI job itself (Step 6).

### Step 6 · Quality gates and CI
- `make lint`, `make test`, `make fmt` cover all three languages.
- `lefthook`: format and lint staged files; reject commit messages that break the convention in [README.md](README.md).
- GitHub Actions: lint, test, and build per service, with path filters; `govulncheck`, `pip-audit`, `pnpm audit`; secret scanning with `gitleaks`; Docker image builds.
- Dockerfiles for the Python and Next.js services: multi-stage, minimal base, non-root user, pinned base image versions (the Go one exists since Step 2).
- **Verify:** a deliberately broken lint, a failing test, a bad commit message, and a planted fake secret are each rejected.

## Security (from [security.md](security.md))

| Threat | Applied here |
|---|---|
| T12 Internal service exposure | Only `web` and `api` publish ports. `ai` and the data stores are on the private compose network; data stores bind to localhost for development only |
| T13 Supply chain | Lockfiles committed, pinned images, vulnerability scans in CI |
| T15 Secrets management | `.env` git-ignored, `.env.example` with placeholders only, `gitleaks` in hooks and CI |
| T16 Error and log leaks | Generic error body with a request ID; no stack traces in responses; config values never logged |

## Scenarios to test

- A required environment variable is missing or malformed: the service refuses to start with a clear message.
- Postgres or Redis is down: `/healthz` is 200, `/readyz` is 503, and the status page shows "degraded".
- The AI service is down: the Go API still responds and reports it as unavailable.
- The Go API is down: the web page shows an error state instead of crashing.
- A port is already in use: startup fails with a clear message.
- SIGTERM during a request: the request finishes, then the process exits.
- A fresh clone on a clean machine: `make dev` works with only Docker, Go, uv, Node, and pnpm installed.

## Done when

- [ ] `make dev` starts the full stack from a fresh clone.
- [ ] The status page shows both services healthy, via the Go API.
- [ ] `make lint` and `make test` pass locally and in CI.
- [ ] A badly formatted commit message is rejected.
- [ ] Editing a contract and running `make gen` updates types in all three languages.
- [ ] Every scenario above has a test or a recorded manual check.

## Needed from the developer

- Create a private GitHub repository and add it as `origin` (Step 6 needs it; Steps 1–5 do not).
