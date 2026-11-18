# Database migrations

The schema lives here as plain SQL, applied by [goose](https://github.com/pressly/goose) through the
`migrate` command (`services/core/cmd/migrate`). The SQL is embedded in that binary, so production
runs migrations from the `readmely/migrate` image and needs nothing else. The design and the reasons
are in [plans/phase-1-database.md](../../../plans/phase-1-database.md).

## Everyday commands

| Command | What it does |
|---|---|
| `make up` | start the data stores, apply migrations, set the service roles' passwords |
| `make db-status` | list migrations and whether each is applied |
| `make db-new NAME=add_thing` | create an empty, correctly numbered migration file |
| `make db-up` | apply pending migrations (and set role passwords) |
| `make db-down` | revert the latest migration (development only) |
| `make db-reset` | revert everything and apply again; **deletes local data** (development only) |
| `make db-seed` | load fictional users, projects, jobs and credits (development only, safe to repeat) |

`down`, `reset` and `seed` refuse to run when `APP_ENV` is `production` (`seed` also refuses staging).

## Adding a migration

1. `make db-new NAME=add_thing`, then write `-- +goose Up` and `-- +goose Down`. Wrap function bodies
   in `-- +goose StatementBegin` / `-- +goose StatementEnd`.
2. If the Go code needs it, add or change queries in `services/core/internal/db/queries/*.sql`, then run
   `make gen`. It rewrites `services/core/internal/db/store/`; commit the result. `make gen-check`
   (and CI, and the pre-commit hook) fail if it is stale.
3. If the table is shared with Python, add or update its Pydantic model (see `services/ai/app/jobs.py`)
   and the drift test.
4. `make test-integration` runs the schema, queue, ledger and seed tests against real Postgres. The
   conventions test fails if a new table breaks the rules below.

## Rules

- **Never edit a migration after it has merged.** Add a new one. Destructive changes take two steps:
  stop using the column, then drop it in a later migration.
- Production only goes forward. `Down` exists for development and is exercised by tests (up, down to
  zero, up again).
- Each migration runs in one transaction; a failure leaves nothing half applied.
- Keys are `uuid` with `DEFAULT uuid_generate_v7()`; times are `timestamptz`; names are `snake_case`
  with constraints named `<table>_<columns>_<kind>`; statuses are `text` with a named `CHECK`; every
  foreign key says what happens on delete and has an index that starts with its columns; every table
  with `updated_at` gets the `set_updated_at` trigger. `internal/schema` enforces all of this.
- Deleting data really deletes it (no soft-delete flags). The credit ledger is the one exception: it
  is append-only, and deleting an account only clears `user_id` and `job_id` on its rows.
- No secret in any migration or seed file.

## Roles

Migrations run as the owner. The services connect as limited roles created by the first migration:

| Role | May | May not |
|---|---|---|
| `readmely_app` (Go API and workers) | read and write rows; call `credit_reserve` | create, alter or drop anything; change or delete ledger rows |
| `readmely_ai` (Python AI service) | connect | everything else until a feature is granted what it needs |

A table created by a migration is readable and writable by `readmely_app` automatically; a migration
that needs stricter rules revokes what it must (the ledger does). The AI role gets nothing by default:
grant it table by table in the migration that needs it. `make db-up` sets both passwords from
`APP_DB_PASSWORD` and `AI_DB_PASSWORD`.

## The job queue

Workers use the queries in `internal/db/queries/jobs.sql`:
`EnqueueJob` (idempotent per user and key) → `ClaimJob` (oldest runnable first, optionally by kind,
`FOR UPDATE SKIP LOCKED`) → `ExtendJobLease` while working → `CompleteJob` or `ReleaseJob` (retry with
backoff, or fail when attempts run out). A worker that stops renewing its lease loses the job to
`RequeueExpiredJobs`; every change to a claimed job names the worker, so the old worker's late calls
change nothing.

## The credit ledger

Append-only: a trigger refuses `UPDATE`, `DELETE` and `TRUNCATE` for everyone. Balance is the sum of
`delta` (`credit_balances`). `credit_reserve(user, amount, job, key)` holds credits under a per-user
lock, fails with `CR001` when the balance is too small and `CR002` when the key was reused for a
different request, and returns the original entry on an identical replay.
