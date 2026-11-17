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

`make help` lists every target, including `lint`, `test`, `test-e2e` (browser tests), `test-integration` and `audit`.

The web app is at <http://127.0.0.1:3000> and shows the status of the API and AI service. The API serves `/healthz` publicly; `/readyz` is on an internal-only port (`INTERNAL_ADDR`).
The AI service needs `AI_SERVICE_TOKEN` in `.env` (generate with `openssl rand -hex 32`).

## Security

Never commit `.env`. Threats and the per-feature checklist are in [`plans/security.md`](plans/security.md).
