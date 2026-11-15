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
| Local stack | Docker Compose: Postgres 17 with pgvector, Redis, MinIO |
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
- `infra/docker-compose.yml` with Postgres (pgvector image), Redis, and MinIO, each with a health check and a named volume.
- Data stores bind to `127.0.0.1` only, with credentials read from `.env`.
- **Verify:** `make up` starts all three and reports healthy; `make down` stops them; `CREATE EXTENSION vector` succeeds.

### Step 2 · Go core service
- One module, three binaries. `ingest` and `render` only start, log, and exit cleanly on a signal.
- `api`: config from environment with validation, structured JSON logs with a request ID, panic recovery, timeouts, graceful shutdown.
- `GET /healthz` (process is up) and `GET /readyz` (Postgres and Redis reachable).
- **Tests:** config validation (missing and malformed values), health handlers, readiness when a dependency is down, shutdown.

### Step 3 · Python AI service
- FastAPI app with `GET /healthz` and `GET /readyz`, settings via Pydantic, structured logs.
- Worker entry point that starts and exits cleanly.
- Not published to the host; reachable only on the compose network.
- **Tests:** settings validation, health endpoints, readiness when Postgres is down.

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
- Dockerfiles: multi-stage, minimal base, non-root user, pinned base image versions.
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
