# Readmely

Turns a codebase into a README, architecture diagrams, a context pack for AI tools, a resume, and a portfolio.

**Status:** early development (Phase 0, foundations). See [`plans/README.md`](plans/README.md) for the plan and progress, and [`project-description.md`](project-description.md) for the product.

## Stack

| Part | Tech | Folder |
|---|---|---|
| Core API and workers | Go | `services/core` |
| AI service (all LLM work) | Python, FastAPI | `services/ai` |
| Web app | Next.js 16, React 19, Tailwind | `apps/web` |
| Local infrastructure | Postgres + pgvector, Redis, RustFS (S3) | `infra` |

## Prerequisites

Docker with Compose, Go 1.27+, [uv](https://docs.astral.sh/uv/) (Python 3.13), Node 24 with [pnpm](https://pnpm.io/) 12, and `make`.

## Run it locally

```sh
cp .env.example .env     # then change every password; use only letters, digits, - and _
make up                  # start Postgres, Redis and RustFS, bound to 127.0.0.1
make verify              # check credentials, connectivity and that ports are local-only
make up-app              # optional: run the API, AI service and web app in containers
make run-api             # or run the Go API on the host
make run-web             # and the web app on the host (dev mode)
```

Run `make hooks` once after cloning: it enables the git hooks in `.githooks/`, which reject commit messages that break the convention below and check the staged files (format, lint, contracts, secrets) before each commit.

The database schema is versioned in `services/core/migrations/` (goose, plain SQL). `make up` starts the data stores and applies the migrations; `make db-status`, `make db-new NAME=...`, `make db-down` and `make db-reset` manage them. The services log in as two limited roles (`readmely_app`, `readmely_ai`) that cannot change the schema; their passwords are `APP_DB_PASSWORD` and `AI_DB_PASSWORD` in `.env` (see `.env.example`, at least 16 characters, letters, digits, `-` and `_` only).

API shapes live in [`contracts/`](contracts/README.md) as OpenAPI specs. `make gen` regenerates the Go, Python and TypeScript types from them (output is committed), `make gen-check` fails if generated code is out of date, and `make contracts-check` validates the specs.

`make help` lists every target, including `lint`, `test`, `test-e2e` (browser tests), `test-integration` and `audit`.

The web app is at <http://127.0.0.1:3000> and shows the status of the API and AI service. The API serves `/healthz` publicly; `/readyz` is on an internal-only port (`INTERNAL_ADDR`).
The AI service needs `AI_SERVICE_TOKEN` in `.env` (generate with `openssl rand -hex 32`).

### Signing in locally

Go to <http://127.0.0.1:3000/login>, enter any email address, and read the six-digit code from the API log (`docker compose --env-file ./.env -f infra/docker-compose.yml --profile app logs api`; or the terminal when you use `make run-api`). The development mailer writes emails to the log instead of sending them, and the API refuses to start in production until a real mailer is configured. Use Chromium or Firefox: the session cookie is `Secure`, which they accept over `http://127.0.0.1` and Safari may not.

`make test-e2e-stack` signs in through a real browser against the running stack (`make up-app` first; once, `cd apps/web && pnpm exec playwright install chromium`).

Settings (see `.env.example` and the compose file): `AUTH_CODE_KEY` (required), `WEB_ORIGIN` (the one origin allowed to change state), `MAILER` (`log` or `file`, development only), `TRUSTED_PROXIES` (addresses whose `X-Forwarded-For` the API believes, set it to the web server's network when the web server sits behind a load balancer), and for the web server `API_PUBLIC_URL` and `WEB_BEHIND_PROXY` (`true` only when a trusted proxy in front sets `X-Forwarded-For`; otherwise the header is dropped and every visitor shares one rate-limit address).

## Security

Never commit `.env`. Threats and the per-feature checklist are in [`plans/security.md`](plans/security.md).
