# Readmely plans

Every feature is built human-in-the-loop. Nothing is implemented without an approved plan.

## Per-feature loop

1. **Discuss** the flow in chat. Open questions are settled here.
2. **Draft** `plans/<phase>/<feature>.md` (status: `Draft`) covering flow, data, API contract, security risks, scenarios, and test plan.
3. **Approve.** The developer reviews and marks it `Approved`.
4. **Implement** in small steps on a feature branch. Each step is reviewed before the next.
5. **Test** against the scenario list: unit, integration, contract, security fixtures, and AI evals where relevant.
6. **Sign off.** The developer marks the plan `Done` and the branch is merged.

## Plan status

| Plan | Status |
|---|---|
| [00-architecture.md](00-architecture.md) | Approved |
| [security.md](security.md) | Approved |
| [phase-0-foundations.md](phase-0-foundations.md) | Approved · Steps 1–4 done · Step 5 awaiting sign-off |
| [audit-2026-11-17.md](audit-2026-11-17.md) | Done · fixes merged (fix-audit-items.md) |

## Git conventions

**Branches:** `<type>/<short-name>`, for example `feat/auth-totp`, `fix/credit-race`, `docs/architecture-plan`. One branch per feature; `main` only receives reviewed merges.

**Commits:** `<type>(<scope>): <summary>`, summary in the imperative, under 72 characters.

| Type | Use for |
|---|---|
| `feat` | A new feature or user-visible behaviour |
| `fix` | A bug fix |
| `docs` | Plans and documentation only |
| `test` | Adding or changing tests only |
| `refactor` | Code change with no behaviour change |
| `perf` | Performance improvement |
| `security` | A security hardening or vulnerability fix |
| `build` | Dependencies, Dockerfiles, build tooling |
| `ci` | CI pipeline changes |
| `chore` | Anything else (misc.): config, housekeeping |

**Scopes:** `web`, `core`, `ai`, `layout`, `db`, `contracts`, `infra`, `plans`.

Examples: `feat(core): add TOTP enrolment endpoint` · `fix(db): make credit debit atomic` · `chore(infra): bump postgres image`.
