# Security baseline

**Status:** Draft, awaiting review · **Last updated:** 2026-11-15

Every feature plan has a "Security" section that lists which threats below apply and how they are tested. A feature cannot be marked Done with an untested applicable threat.

## Principles

1. **Repository content is hostile input.** It is never executed, and it is never trusted as instructions to an LLM.
2. **Every read and write is scoped to an owner.** There is no query without a user or project filter.
3. **Secrets stay in one place.** Provider keys live only in `ai`; GitHub credentials live only in `core` and `ingest`; the browser gets none.
4. **Generated output is untrusted too.** It is sanitised before it is stored or shown.
5. **Fail closed.** If a check cannot run, the job fails.

## Threats and mitigations

| ID | Threat | Mitigation | Phase |
|---|---|---|---|
| T1 | **Malicious repository or ZIP:** path traversal, symlink escape, zip or git bombs, huge files, submodules pointing at internal hosts, git hooks, LFS | No code execution. Symlinks and submodules are not followed. Hooks and LFS disabled. Caps on total size, file count, file size, and clone time. Ingestion runs in a container with no outbound network except the git host, a read-only root, and CPU and memory limits | 4 |
| T2 | **Prompt injection** from code, comments, or READMEs | Repository text is passed as data, clearly delimited. LLM tools reach only the current project. No tool can send data out. Outputs are schema-validated and sanitised. Diagram nodes need evidence | 5–8 |
| T3 | **Secret leakage** to LLMs, embeddings, logs, or context packs | Secret scan and redaction in `ingest` before content leaves it. `.env` files excluded. Environment variable names only. Logs never contain code or prompts in production | 4–6 |
| T4 | **XSS and content injection** through SVG, Markdown, or generated text | SVG built from our own components and sanitised; shown as `<img>`. Markdown sanitised on render. User content served from a separate origin. Strict Content-Security-Policy | 6–8 |
| T5 | **Cross-user data access** (IDOR) | Ownership check in one shared layer in `core`. Qdrant queries always filter on `project_id`. Object storage reached only through short-lived signed URLs. Tests try every endpoint as another user | 2 onwards |
| T6 | **Authentication attacks:** credential stuffing, session theft, OAuth CSRF, account-linking takeover, 2FA bypass | Argon2id if passwords are used. OAuth `state` and PKCE. Accounts link only on a provider-verified email. HttpOnly, Secure, SameSite cookies; session rotation on login and 2FA. TOTP secrets encrypted; recovery codes hashed; rate limits and lockout on login and code entry; step-up for sensitive actions | 2 |
| T7 | **CSRF** | SameSite cookies plus a CSRF token on state-changing requests. Strict CORS allow-list | 2 |
| T8 | **GitHub App abuse:** stolen tokens, forged or replayed webhooks | Minimum read permissions. Installation tokens fetched on demand and never stored. Webhook HMAC check, delivery-ID replay guard. App private key in a secret store | 3 |
| T9 | **SSRF** through repository URLs, Markdown image links, or SVG rasterising | URL allow-list for git hosts. Private and link-local addresses blocked after DNS resolution. The rasteriser loads no external resources | 3, 8 |
| T10 | **Cost abuse and denial of service** | Per-user rate limits and concurrency caps. Repository size limits by plan. Per-job token budget. Global spend circuit breaker. Sign-up abuse controls | 3, 5 |
| T11 | **Credit races and double spending** | Append-only ledger. Credits reserved in one transaction before a job, settled or refunded when it ends. Idempotency keys on every job | 5 |
| T12 | **Internal service exposure** | `ai`, workers, and data stores on a private network. Service-to-service tokens. No default credentials | 0 |
| T13 | **Supply chain** | Pinned dependencies and lockfiles. `govulncheck`, `pip-audit`, `npm audit`, and image scanning in CI. Minimal base images, non-root users | 0 |
| T14 | **Data retention and deletion** (DPDP Act, GDPR) | Raw repository snapshots deleted after ingestion. Deleting a project removes rows, vectors, objects, and cache entries, verified by a test. Providers used with no-training terms | 3 onwards |
| T15 | **Secrets management** | No secrets in the repository. `.env.example` only. Secrets from the host's secret store in production. Key rotation procedure documented | 0 |
| T16 | **Information leaks in errors and logs** | Generic client errors with a request ID. Structured logs with redaction. No stack traces to clients | 0 |

## v2 threats (recorded now, designed later)

| ID | Threat | Mitigation |
|---|---|---|
| T17 | **LaTeX compilation:** shell escape, file reads, infinite loops | Shell escape off, restricted file access, no network, timeout, throwaway container |
| T18 | **Payment webhooks:** forgery, replay, plan tampering | Signature verification, idempotent handlers, plan state changed only by verified events |
| T19 | **BYOK key storage** | Envelope encryption, decrypted only in `ai`, never returned to the browser, redacted from logs |
| T20 | **MCP endpoint tokens** | Per-project scoped tokens, revocable, rate-limited, read-only |
| T21 | **Public portfolio pages** | Sanitised content, separate origin, abuse reporting |

## Per-feature security checklist

- [ ] Which threats (T-numbers) apply?
- [ ] What new input does this feature accept, and how is each field validated?
- [ ] What data does it return, and who is allowed to see it?
- [ ] Does anything reach an LLM? Is it redacted and delimited?
- [ ] Is any output rendered in a browser? How is it sanitised?
- [ ] What are the rate and size limits?
- [ ] Which tests prove each mitigation?
