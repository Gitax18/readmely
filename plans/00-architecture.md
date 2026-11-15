# 00 · Architecture

**Status:** Draft, awaiting review · **Last updated:** 2026-11-15

This is the top-level plan. It records the agreed decisions, the service boundaries, and the phase order. Feature flows are **not** designed here; each gets its own plan after discussion.

## 1. Decisions

| # | Decision | Status |
|---|---|---|
| D1 | **Go** for the core backend, **Python** for all gen-AI work, **Next.js** for the frontend | Agreed |
| D2 | Go ↔ Python: **Postgres-backed job queue** for async work, **internal HTTP** for synchronous calls (diagram tools, chat streaming) | Agreed |
| D3 | **Dedicated vector database**, free to run. Proposed: **Qdrant** (open source, self-hosted in Docker) | Agreed in principle; product choice to confirm |
| D4 | **Own auth service** in Go with **2FA**, plus login with **Google** and **GitHub** | Agreed; details open (see §9) |
| D5 | **MVP:** GitHub import, ingestion, context pack, README, overview diagram. Resume, portfolio, and payments are v2 | Agreed |
| D6 | Build **feature by feature**: backend and its frontend together | Agreed |
| D7 | Hosting on **DigitalOcean or AWS** (undecided). Everything ships as Docker images; Kubernetes is added only if needed | Agreed |
| D8 | LLM access through one **OpenAI-compatible client**; providers at launch are **OpenAI and xAI** | Agreed |
| D9 | Git: feature branches and typed commits (see [README.md](README.md)) | Agreed |
| D10 | **Code graph in Postgres** (`graph_nodes`, `graph_edges`), behind a Go `GraphStore` interface so it can move to a graph database later. No dedicated graph database in the MVP; revisit after release | Agreed |

## 2. Services

| Service | Language | Owns | Public? |
|---|---|---|---|
| `web` | Next.js (TypeScript) | Dashboard, editors, public pages. Holds no secrets; calls only `core` | Yes |
| `core` API | Go | Auth, sessions, 2FA, users, projects, plans and credits, GitHub App, webhooks, rate limits, job dispatch, file access | Yes |
| `ingest` worker | Go | Clone, filter, secret scan, tree-sitter parsing, fact extraction, import graph. No LLM calls | No |
| `render` worker | Go | Diagram layout input/output, SVG components, geometric lint, rasterising, exports | No |
| `layout` sidecar | Node | ELK.js layout only | No |
| `ai` service | Python | LLM client, summaries, embeddings, retrieval, README and context pack writing, diagram IR, planning, and vision critique, evals | No |

**Boundary rules**
- Only `core` and `web` are reachable from the internet.
- Only `core` and the Go workers write business tables (users, projects, credits). `ai` writes only its own results and job status.
- Only `ai` talks to LLM providers. Only the Go workers touch raw repository files.
- `ai` reaches repository content through Go-backed internal tools scoped to one project, never through the filesystem.

## 3. Communication

- **Async jobs (default):** a `jobs` table in Postgres. Workers claim rows with `SELECT … FOR UPDATE SKIP LOCKED`. Every job has an idempotency key, an attempt count, a timeout, and a dead-letter state. Go and Python workers consume different job types from the same table.
- **Sync calls:** internal HTTP with JSON, authenticated with short-lived service tokens. Used when `ai` needs a tool result immediately (`get_context_pack`, `read_file`, `submit_ir`, `plan_views`, `render_view`) and for streaming.
- **Contracts:** OpenAPI for HTTP and JSON Schema for job payloads, the diagram IR, and the context pack, all in `contracts/`. Go and Python types are generated from them, so the two sides cannot drift.
- **Progress to the browser:** `core` exposes job status over server-sent events.

## 4. Data stores

| Store | Used for |
|---|---|
| **Postgres** | Users, auth, projects, jobs, credit ledger, file and symbol index, code graph (as node/edge tables), generated document metadata |
| **Qdrant** | Embeddings of summaries and code chunks. Every point carries `project_id`, and every query filters on it |
| **Redis** | Rate limits, short-lived caches, locks, OAuth state |
| **Object storage** (S3 API: MinIO locally, DO Spaces or S3 in production) | Repo snapshots (short-lived), SVG/PNG, context pack ZIPs |

Postgres is the source of truth. Qdrant can always be rebuilt from it.

## 5. LLM layer

- One client in `ai` that speaks the OpenAI-compatible API, with a configurable base URL and key per provider. OpenAI and xAI are two configurations of the same client.
- Models are never named in feature code. Features ask for a **tier** (`fast`, `quality`, `vision`, `embedding`), and a config file maps tiers to provider and model.
- Fallback: if the primary provider fails or is rate-limited, the same tier is retried on the other provider.
- Every call records provider, model, tokens, cost, and project, which feeds the credit ledger and a global spend circuit breaker.
- Structured outputs are enforced with JSON Schema and validated again on our side.
- To verify in Phase 5: which provider serves embeddings and the vision tier.

## 6. Repository layout

```
apps/web/              Next.js
services/core/         Go module: cmd/api, cmd/ingest, cmd/render, internal/...
services/ai/           Python: FastAPI app + job workers
services/layout/       Node ELK sidecar
contracts/             OpenAPI + JSON Schemas
infra/                 docker-compose, Dockerfiles, sandbox profiles
plans/                 plan files
testdata/repos/        fixture repositories, including malicious ones
```

## 7. Deployment

- **Local:** one `docker compose up` starts Postgres, Qdrant, Redis, MinIO, and all services.
- **Production (first):** the same images on a single DigitalOcean droplet or AWS instance, behind a reverse proxy with TLS. Managed Postgres if the budget allows.
- **Later:** Kubernetes manifests when one host is no longer enough.
- **Sandboxing:** the ingest worker handles untrusted repositories, so it runs in a locked-down container (no outbound network except git fetch, read-only root, resource limits). The exact mechanism is decided in the ingestion plan once hosting is fixed.

## 8. Phases

Each phase is a set of features. Each feature follows the loop in [README.md](README.md) and includes its frontend.

| Phase | Scope |
|---|---|
| **0 · Foundations** | Monorepo skeleton, docker-compose, CI, linters, test frameworks, contracts tooling |
| **1 · Database** | Entity model, schema, migrations, generated Go and Python models, seed data |
| **2 · Auth and account** | Sign-up, login, Google and GitHub login, 2FA, sessions, profile, settings UI |
| **3 · Projects and GitHub import** | GitHub App, repository picker, project CRUD, job queue, progress UI |
| **4 · Ingestion** | Clone, filter, secret scan, parsing, facts, code graph |
| **5 · AI foundation** | LLM client, tiers, summaries, embeddings, retrieval, usage metering, credit ledger |
| **6 · Context pack** | Build, presets, formats, download |
| **7 · README** | Generation, section regeneration, editor, health score |
| **8 · Overview diagram** | The pipeline in `ai-based-image-generation.md` |
| **MVP release** | Hardening, security review, deployment |
| **v2** | Resume and LaTeX sandbox, portfolio, payments, Champ extras |

The credit ledger is in Phase 5, before payments, because usage has to be capped as soon as AI calls cost money.

## 9. Testing strategy

- **Unit tests** for every package (Go `testing`, pytest, Vitest).
- **Integration tests** against real Postgres, Qdrant, Redis, and MinIO containers.
- **Contract tests** so Go and Python agree on every schema.
- **Security fixtures** in `testdata/repos/`: zip-slip, symlink escape, oversized files, planted secrets, prompt-injection READMEs.
- **AI evals** with recorded provider responses for CI, plus a scored eval set run on every prompt change.
- **End-to-end tests** (Playwright) for each completed feature.
- A feature is not done until every scenario in its plan has a test.

## 10. Open questions

1. **Vector database:** confirm Qdrant.
2. **Auth method:** email and password with TOTP 2FA, or passwordless email codes with TOTP? Settled in the Phase 2 auth discussion.
3. **Diagram output:** `project-description.md` §2.4 says diagrams are generated as Mermaid/PlantUML/D2. The pipeline guide generates a structured graph and renders our own SVG, with D2/Mermaid as exports only. Confirm the guide wins, and the description is updated.
4. **Hosting:** DigitalOcean or AWS. Needed before the ingestion sandbox is designed (Phase 4).
5. **BYOK and Claude:** the description lists Claude and Gemini keys for Champ users. With OpenAI and xAI at launch, other providers need an adapter later. Confirm this is v2.
