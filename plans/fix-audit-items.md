# Fix plan: audit items (branch `fix/audit-items`)

**Status:** Approved 2026-11-18 · **Source:** [audit-2026-11-17.md](audit-2026-11-17.md)
One commit per item. Run `make lint test test-integration audit` after each.

## 1 · A9 Go standard-library vulnerabilities (High, do first)
- `services/core/go.mod`: `go 1.27.2` → `go 1.27.4` (Go auto-downloads the toolchain).
- `infra/docker/core.Dockerfile`: `golang:1.27.2-alpine` → `golang:1.27.4-alpine` (check the tag exists first).
- **Done when:** `make audit` reports 0 Go vulnerabilities; `make test` and `make docker-build` pass; the api container is healthy.
- Commit: `security(core): upgrade Go to 1.27.4 for GO-2026-6859 and GO-2026-6809`

## 2 · A1 Public `/readyz` on the Go API (High)
- Serve `/readyz` on a second, internal-only listener (`INTERNAL_ADDR`, default `127.0.0.1:8081`, not published in compose); the public router keeps `/healthz` only.
- **Tests:** `/readyz` absent on the public port (404); present on the internal one; both listeners shut down gracefully.

## 3 · A2 URL-unsafe passwords (Medium)
- `make verify` fails if a `.env` password or token contains anything other than letters, digits, `-` or `_` (the `.env.example` note is already done).

## 4 · Documentation fixes (A3–A5, A7, A8)
- A3: re-price `project-description.md` §4.1 on OpenAI and xAI rates (from their own pricing pages).
- A4: align §2.10 sign-up text with D4 (Google + GitHub + 2FA).
- A5: note at the top of `ai-based-image-generation.md` that LLM orchestration runs in Python (D1).
- A7: already done. A8: add a short root `README.md`.

A6 (`session-id.txt`) is already done.
