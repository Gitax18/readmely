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

### Step 4 · Next.js shell
- App Router project with Tailwind and shadcn/ui, strict TypeScript.
- One status page showing API and AI service health, fetched **through the Go API** (`GET /api/v1/status`), never directly from the AI service.
- Security headers (Content-Security-Policy, `X-Content-Type-Options`, `Referrer-Policy`, frame denial).
- **Tests:** unit test for the status component (healthy, degraded, API unreachable); one Playwright test that loads the page.

### Step 5 · Contracts and type generation
- `public.yaml` (browser ↔ core) and `internal.yaml` (core ↔ ai), starting with the health and status endpoints.
- `make gen` produces Go, Python, and TypeScript types; generated code is committed.
- **Tests:** CI fails if `make gen` produces a diff, which proves contracts and code cannot drift.

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
