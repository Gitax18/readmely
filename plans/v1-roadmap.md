# Version 1 roadmap (fast-forward build)

**Status:** Approved (2026-11-24) · **Last updated:** 2026-11-24

The shortest safe path from today (sign-in is done) to a first release of Readmely: import a public GitHub repository, analyse it, and get a README, a context pack and an overview diagram. What a user sees in version 1 is described in [v1-brief.md](v1-brief.md). The long-term shape stays in [00-architecture.md](00-architecture.md); this plan only compresses the order and the scope.

## 1. How the fast-forward build works

| Topic | Rule |
|---|---|
| Approval | You approve this roadmap and the brief once. After that each phase is built on its own branch without a separate plan sign-off. |
| When I stop | Only for a decision that is yours: money, accounts, credentials, hosting, product wording. Section 6 lists them with a recommendation and the phase that needs each. |
| Tests | No new tests are written while version 1 is built. The existing suite and every CI gate stay and must pass; `main` still needs a green `ci-ok`. A test-backfill step in Phase 9 covers the security-critical code. |
| Coverage gates | The web gate (90% lines) and the Python gate (`fail_under = 90`) would fail on code without tests. They are lowered in one clearly labelled commit (`ci: lower the coverage gates while version 1 is built`) and restored in Phase 9. |
| Still enforced | Linters, type checks, `make gen-check`, the contract check, dependency audits, the secret scan, image scans, and the commit-message check. Contracts-first stays: it is cheap and it keeps Go, Python and TypeScript in agreement. |
| Verification | Instead of tests, I run each feature in the real stack (`make up-app`) and drive it in a browser before opening the pull request, and the PR body lists what I checked. |
| Git | One branch and one pull request per phase, typed commits (`feat(core): ...`), no work-in-progress messages, a PR body with Summary and "How I checked it". Merge commits, as so far. A recruiter reading the history should be able to follow the product being built. |
| Security | Fast does not mean unsafe. These stay in every phase: the repository is untrusted input and is only touched inside the locked-down ingest container, secrets are removed before any text reaches a model, repository text is treated as data and never as instructions, every endpoint checks ownership, and credits cap spend before a model is called. |

## 2. Scope of version 1

In: sign-in (done), account deletion, projects from a public GitHub URL, analysis, README generation and editing, context pack download, overview diagram (SVG and PNG), credits with a free grant, landing page, legal pages, deployment.

Out (version 1.1 and v2): private repositories and the GitHub App, TOTP and recovery codes, payments and plans, resume, portfolio, team features, automatic regeneration on push, chat with a repository, the vision critique pass for diagrams.

## 3. Phases at a glance

Sizes are relative (S, M, L), not calendar estimates. Each phase ends with something you can open and use.

| # | Phase | Size | You can then | Branch |
|---|---|---|---|---|
| 2c-lite | Account essentials | S | Delete your account and sign out other devices | `feat/account-essentials` |
| 3 | Projects and import | M | Sign in, see a dashboard, import a repository and watch a progress bar | `feat/projects-import` |
| 4 | Ingestion | L | See what Readmely understood about a repository | `feat/ingestion` |
| 5 | AI foundation and credits | L | See credits, and have summaries written for a project | `feat/ai-foundation` |
| 6 | Context pack | M | Download a context pack for an AI coding tool | `feat/context-pack` |
| 7 | README | L | Generate, edit and download a README (first full demo) | `feat/readme-generation` |
| 8 | Overview diagram | L | Generate and download an architecture diagram | `feat/overview-diagram` |
| 9 | Release | M | Use it on the internet at your own domain | `feat/release` |

Dependencies are linear except that 6 and 7 could swap; the context pack goes first because the README writer reads from it. Phase 7 is the first demo you can show someone, so it is the milestone to protect.

### Progress and changes from the plan

| Phase | State | What differs from the text below |
|---|---|---|
| 2c-lite | Done (PR #18) | Deletion is two `POST` steps (`/v1/me/deletion/start`, then `/confirm`) instead of `DELETE /v1/me`, which avoids a body on `DELETE`. Moving the status panel to `/status` is deferred to Phase 9 with the landing page. |
| 3 | Done (PR #19) | Progress is polled every two seconds instead of streamed with server-sent events: the same-origin bridge buffers responses and caps their size, so a stream would need a second path out of the web server. A `POST /v1/projects/preview` endpoint checks a repository before anything is created. Signing in still lands on `/account`; the post-login destination moves to the dashboard with the landing page in Phase 9. The worker's analysis steps are a placeholder until Phase 4. |
| 4 | Done (PR #20) | Parsing: Go uses the standard library parser; Python, JavaScript and TypeScript are read line by line with patterns instead of tree-sitter, which needs native code in the worker image and adds little for what the facts and the graph use. The repository is read from a GitHub archive pinned to one commit, in memory, so there is no clone and nothing from it touches the disk, which is why no extra sandbox layer was needed beyond the existing locked-down container. Coverage gates for the web app are 55/50/50/55 while tests are skipped. |
| 5 | Built, in review | Split in two: credits shipped first (PR #21: a one-time grant of 100, balance and history, `/usage`) and the model client second. One OpenAI provider with a configurable model and price (default `gpt-4o-mini`), so there is no fallback provider. The summary is one structured call per analysis (README, facts, entry points and the most imported files go in; headline, overview, how it works and key files come out) instead of per-file and per-directory summaries. **Cut for now:** pgvector retrieval and `chunks`, because the summary needs no embeddings; they return in Phase 6 if the context pack needs them. One credit is half a US cent: analysis holds 10 credits and is settled to what the call cost, at least 1. A $3 daily cap (`LLM_DAILY_CAP_USD`) stops all calls. The summary arrives after the analysis, which is ready at once; the page shows it is being written, or why it was not. |

## 4. The phases

### Phase 2c-lite · Account essentials

**Goal:** the account controls a real product needs, without TOTP.

- `DELETE /v1/me` removes the user, their projects and jobs by cascade and anonymises ledger rows (rules already decided in the Phase 1 plan). Confirmation needs a fresh emailed code (a step-up), then clears the session and shows a goodbye page.
- `POST /v1/me/sessions/revoke-others` signs out every other device.
- `/account` gains a danger zone with the delete flow and a "sign out other devices" button.
- Moves the status panel from `/` to `/status` so `/` can become the landing page later.

**Cut:** TOTP, recovery codes, email change.
**Data:** none new. **Done when:** deleting an account leaves no row behind for that user except anonymised ledger entries (checked by hand in `psql`).

### Phase 3 · Projects and import

**Goal:** a signed-in person can create a project from a public GitHub URL and watch it being worked on.

- **Data:** `projects` gains `repo_url`, `repo_owner`, `repo_name`, `default_branch`, `commit_sha`, `visibility`; a unique key on `(user_id, repo_owner, repo_name)`.
- **API (core):** `POST /v1/projects` (URL in, validated against `github.com/<owner>/<repo>` only, then the repository is checked through the public GitHub API: exists, is public, size under the limit), `GET /v1/projects`, `GET /v1/projects/{id}`, `DELETE /v1/projects/{id}`, `POST /v1/projects/{id}/analyse` (enqueues the ingest job), `GET /v1/projects/{id}/events` (server-sent events with job progress), `GET /v1/jobs/{id}`.
- **Worker runtime:** a job runner in `cmd/ingest` that claims jobs from the existing queue, heartbeats, retries with backoff and reports progress. This is the engine every later phase uses.
- **Web:** app shell (top bar with account menu and credit balance, project nav), `/dashboard`, `/projects/new`, `/projects/[id]` with a live progress view, empty states and error states.
- **Limits:** 10 projects per user, one running job per project, repository size cap, per-user rate limit on creating projects.

**Cut:** private repositories, the GitHub App, repository search (version 1 takes a pasted URL), zip upload.
**Done when:** pasting `https://github.com/owner/repo` creates a project, the job runs a placeholder step end to end, and the progress bar moves in the browser.

### Phase 4 · Ingestion

**Goal:** turn an untrusted repository into trustworthy facts.

- **Sandbox:** the ingest container runs as non-root with a read-only root file system, no capabilities, CPU, memory, time and disk limits, and a scratch volume that is deleted after each job. Egress is limited to `github.com` by network configuration where the host supports it; the exact mechanism is finalised with the hosting choice (section 6).
- **Steps:** shallow clone at a pinned commit, safe extraction rules (no symlinks out, no paths outside the root, size and file-count caps), filtering (vendored, generated, binary and huge files), a secret scan that redacts findings before anything is stored, language detection, tree-sitter parsing for Go, Python, TypeScript and JavaScript (other languages are indexed as plain files), manifest and dependency reading (`package.json`, `go.mod`, `pyproject.toml`, `requirements.txt`, `Dockerfile`, compose files, CI files), entry-point and framework detection, an import graph, and a directory summary.
- **Data:** `files`, `symbols`, `graph_nodes`, `graph_edges` (designed in Phase 1), plus a `project_facts` row of JSON for what the UI shows (languages with line counts, frameworks, entry points, scripts, services, environment variables used, license).
- **API:** `GET /v1/projects/{id}/facts`, `GET /v1/projects/{id}/files` (tree, paths only), `GET /v1/projects/{id}/files/content?path=` (redacted text, size-capped; this is also the tool the AI service will call).
- **Web:** project overview tab with languages bar, tech stack chips, entry points, file tree browser, and a "what we skipped and why" note (secrets found are reported as counts and locations, never values).

**Cut:** call-graph precision, languages beyond the four, monorepo package detection beyond workspaces in `package.json` and `go.work`.
**Done when:** analysing a mid-size public repository fills the overview tab, and a planted fake secret in a test repository never appears in the database or the API.

### Phase 5 · AI foundation and credits

**Goal:** one safe, metered way to call a model, and the first thing it is used for.

- **LLM client (Python):** OpenAI-compatible client with tiers (`fast`, `quality`, `embedding`), a config file mapping tiers to provider and model, one automatic fallback to the second provider, structured output with JSON Schema checked again on our side, timeouts, and a hard output cap.
- **Metering:** every call writes `llm_calls` (provider, model, tokens, cost, job). The credit ledger already exists: a job reserves credits before the first call, settles to actual use, refunds on failure. A global daily spend cap stops all calls when reached.
- **Credits:** a one-time grant of 100 credits at sign-up (a `grant` ledger entry, once per user), balance endpoint `GET /v1/me/credits` and history `GET /v1/me/credits/history`.
- **Retrieval:** `chunks` with pgvector, embedded from file summaries and code chunks, always filtered by `project_id`; a retrieval function the generators share.
- **First use:** per-file and per-directory summaries and a project summary, produced by an `summarise` job that runs after ingestion; it makes the overview tab say what the project does in plain English.
- **Prompt-injection posture:** repository text goes into prompts only inside clearly delimited data blocks, the system prompt says it is untrusted data, outputs are schema-validated, and the model has no tools that write anything.
- **Web:** credit balance in the top bar, `/usage` with balance and history, "not enough credits" state.

**Cut:** the scored eval harness (version 1.1), a vision tier, per-user spend alerts.
**Done when:** analysing a project reserves, then settles credits, the ledger shows it, and turning the provider key off makes the job fail cleanly with the credits refunded.

### Phase 6 · Context pack

**Goal:** a downloadable bundle that lets an AI coding tool understand the project.

- **Content:** project overview, architecture notes, directory map, key files with one-line purposes, commands (install, run, test, build) read from scripts and CI, conventions inferred from linters and config, and a dependency summary. Mostly built from facts, with the model used for prose.
- **Presets:** `CLAUDE.md`, `AGENTS.md`, Cursor rules (`.cursor/rules/*.mdc`), and a generic `CONTEXT.md`. A token estimate is shown per preset.
- **Data and API:** `documents` and `document_versions` (kind `context_pack`), `artifacts` for the ZIP in object storage; `POST /v1/projects/{id}/context-pack`, `GET /v1/projects/{id}/context-pack` (latest, with preview), `GET /v1/projects/{id}/context-pack/download?preset=` (ZIP via a short-lived signed link).
- **Web:** "Context pack" tab with preset picker, rendered preview, copy and download.

**Cut:** custom sections, multi-repo packs.
**Done when:** the downloaded `CLAUDE.md` for a known repository correctly states its run and test commands.

### Phase 7 · README

**Goal:** a README worth committing, editable in the browser.

- **Generation:** a planner chooses sections from the facts and the project type (library, web app, CLI, service), then writes each section from the context pack and retrieval. Templates: Standard, Minimal, Detailed. Sections can be regenerated one at a time with an optional instruction ("shorter", "add Docker steps").
- **Health score:** a deterministic checklist score (title and description, install, usage, configuration, license, contributing, badges, structure) with a list of what is missing and a "fix with AI" button per missing item.
- **Data and API:** `documents` (kind `readme`) with `document_versions` for history; `POST /v1/projects/{id}/readme`, `PUT /v1/projects/{id}/readme` (save an edit), `POST /v1/projects/{id}/readme/sections/{key}/regenerate`, `GET .../readme/versions`, `POST .../readme/versions/{id}/restore`.
- **Web:** README tab with section list on the left, Markdown editor and live preview, version history drawer, copy and download.

**Cut:** collaborative editing, pull request creation on GitHub (needs the GitHub App, version 1.1).
**Done when:** a repository goes from pasted URL to a downloaded README with no manual steps, and an edit survives a reload.

### Phase 8 · Overview diagram

**Goal:** a clean architecture diagram, produced by the pipeline in [ai-based-image-generation.md](../ai-based-image-generation.md).

- **Pipeline:** the model writes a structured graph (5 to 9 nodes, typed edges) from the facts and graph; the Node `layout` sidecar runs ELK; our own SVG renderer applies the design system (one colour per category, two font sizes); a geometric lint checks overlaps, text overflow and edge crossings; a failed lint goes back to the graph step once; then export.
- **Data and API:** `artifacts` (SVG and PNG), `POST /v1/projects/{id}/diagram`, `GET /v1/projects/{id}/diagram`, `GET .../diagram/download?format=svg|png`. The README tab gets an "insert diagram" action that adds the image reference and commits nothing to GitHub.
- **Web:** Diagram tab with zoom and pan, regenerate with an optional focus note, download, and copy of the Markdown embed.

**Cut:** the vision critique pass, detail diagrams per sub-flow, D2 and Mermaid export, hand editing.
**Done when:** a known repository produces a diagram with no overlapping boxes, and a malformed model answer is repaired or rejected rather than rendered.

### Phase 9 · Release

**Goal:** it is on the internet, and the repository looks like a product built with care.

- **Product:** landing page (hero, how it works, a real sample output, FAQ), `/privacy` and `/terms`, a 404 page, loading and error boundaries, mobile layouts for every screen, favicon and social card.
- **Operations:** production mailer (so sign-in works), hosting, TLS and domain, a `docker compose` production file with the sandbox settings, backups for Postgres, structured logs, an uptime check, and the production values for `WEB_BEHIND_PROXY` and `TRUSTED_PROXIES`.
- **Hardening:** restore the coverage gates, backfill tests for the code where a bug is a security bug (ownership checks, the sandbox rules, secret redaction, credit accounting), a security review of the branch, dependency and image audits, and the audit items still open from November (F17 to F23).
- **Repository:** a README with screenshots and an architecture diagram generated by Readmely itself, a short architecture write-up, and the plans folder left in place as the design history.

**Done when:** a stranger can sign up on the live site, analyse a public repository and download all three outputs.

## 5. Risks to watch

| Risk | Why it matters | Plan |
|---|---|---|
| Model cost | Each project can cost real money before there are payments | The 100-credit grant, per-job reserve and settle, a global daily cap |
| Untrusted repositories | Cloning arbitrary code is the biggest attack surface | The sandbox rules in Phase 4 are not skipped to save time |
| Diagram quality | The hardest feature; a bad diagram damages the demo | It is last, so a slip delays the release by one phase, not the README |
| Large repositories | Slow analysis and high cost | Size and file caps in Phase 3 and 4; clear "too large" message |
| No tests | Regressions are found by hand | Existing suite stays green; the Phase 9 backfill; I drive each feature in a browser before every pull request |

## 6. Decisions that are yours

| # | Decision | Recommendation | Needed by |
|---|---|---|---|
| 1 | **Approve** this roadmap, the brief, and the fast-forward rules in section 1 | Approve | Now |
| 2 | **LLM accounts:** an OpenAI key and an xAI key (decision D8), with a monthly budget | Start with one provider and a low cap; the second is a fallback | Phase 5 |
| 3 | **Free grant size** | 100 credits at sign-up (about two full projects, see the brief) | Phase 5 |
| 4 | **Hosting** | One DigitalOcean droplet with Docker Compose and Caddy for TLS; simplest to run and to explain | Phase 4 (sandbox) and 9 |
| 5 | **Production mailer** | Resend or Postmark; a few lines of code behind the existing mailer interface | Phase 9 |
| 6 | **Domain name** | Any; needed for the Google and GitHub sign-in callbacks in production | Phase 9 |
| 7 | **Real sign-in providers** | Register the Google client and GitHub OAuth App (README, "Sign in with Google or GitHub") for the demo | Phase 9 |
| 8 | **Private repositories** in version 1 | No, public only; add the GitHub App in 1.1 | Now |
| 9 | **Product copy and name** on the landing page | I draft it, you edit | Phase 9 |
