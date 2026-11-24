# Local development

Everything you need to run, test and change Readmely on your own machine. For the short version,
see the [README](../README.md#quick-start).

## Prerequisites

Docker with Compose, Go 1.27+, [uv](https://docs.astral.sh/uv/) (Python 3.13), Node 24 with
[pnpm](https://pnpm.io/) 12, and `make`.

## Run it

```sh
cp .env.example .env     # then change every password; use only letters, digits, - and _
make up                  # start Postgres, Redis and RustFS, bound to 127.0.0.1
make verify              # check credentials, connectivity and that ports are local-only
make up-app              # optional: run the API, AI service and web app in containers
make run-api             # or run the Go API on the host
make run-web             # and the web app on the host (dev mode)
```

The web app is at <http://127.0.0.1:3000>, and `/status` shows the state of the API and the AI
service. The API serves `/healthz` publicly; `/readyz` is on an internal-only port
(`INTERNAL_ADDR`). The AI service needs `AI_SERVICE_TOKEN` in `.env` (generate it with
`openssl rand -hex 32`).

Run `make hooks` once after cloning. It turns on the git hooks in `.githooks/`, which reject commit
messages that break the [convention](../plans/README.md#git-conventions) and check the staged files
(format, lint, contracts, secrets) before each commit.

`make help` lists every target, including `lint`, `test`, `test-e2e` (browser tests),
`test-integration` and `audit`. `make check` runs everything CI runs that needs no containers.

## Database

The schema is versioned in `services/core/migrations/` (goose, plain SQL). `make up` starts the data
stores and applies the migrations.

| Command | What it does |
|---|---|
| `make db-status` | List migrations and whether they are applied |
| `make db-new NAME=...` | Create an empty migration |
| `make db-down` | Revert the latest migration |
| `make db-reset` | Revert everything and apply again (deletes local data) |
| `make db-seed` | Load fake development data |

The services log in as two limited roles (`readmely_app`, `readmely_ai`) that cannot change the
schema. Their passwords are `APP_DB_PASSWORD` and `AI_DB_PASSWORD` in `.env` (see `.env.example`: at
least 16 characters, letters, digits, `-` and `_` only).

## API contracts

API shapes live in [`contracts/`](../contracts/README.md) as OpenAPI specs. `make gen` regenerates
the Go, Python and TypeScript types from them (the output is committed). `make gen-check` fails if
generated code is out of date, and `make contracts-check` validates the specs.

## Signing in locally

Go to <http://127.0.0.1:3000/login>, enter any email address, and read the six-digit code from the
API log:

```sh
docker compose --env-file ./.env -f infra/docker-compose.yml --profile app logs api
```

With `make run-api`, the code is in that terminal instead. The development mailer writes emails to
the log instead of sending them, and the API refuses to start in production until a real mailer is
configured.

Use Chromium or Firefox. The session cookie is `Secure`; those browsers accept it over
`http://127.0.0.1`, and Safari may not.

`make test-e2e-stack` signs in through a real browser against the running stack. Run `make up-app`
first. Once, run `cd apps/web && pnpm exec playwright install chromium`.

### Sign in with Google or GitHub (optional)

`/login` shows "Continue with Google" or "Continue with GitHub" only for a provider whose
credentials are set. With none set, only emailed codes appear. To try one for real, register an
OAuth client with the provider and put its id and secret in `.env`. Each pair is all or nothing; a
half pair stops the API from starting.

- **Google:** in Google Cloud Console, open *Credentials* and create an OAuth client ID (Web
  application).
  - Authorized redirect URI: `http://127.0.0.1:3000/api/v1/auth/google/callback`.
  - Variables: `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`.
- **GitHub:** open *Settings → Developer settings → OAuth Apps*.
  - Authorization callback URL: `http://127.0.0.1:3000/api/v1/auth/github/callback`.
  - Variables: `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`.
  - This is a separate app from the GitHub App used to import repositories.

Then run `make up-app` again. The redirect URI is always `WEB_ORIGIN` plus that path, so it must
match what you registered.

A provider sign-in links to an existing account only when the provider itself says the email is
verified. "Account" lists connected accounts and can disconnect them; email codes always still
work. The owner gets an email when a provider is connected or disconnected, and when a sign-in comes
from a device they have not used recently. Nothing a provider sends is stored: its tokens live for
one profile request.

The tests cannot approve a sign-in at a real provider. The whole round trip runs in Go against a
fake provider (`make test-integration PKGS=./internal/auth/...`), and the browser tests cover
everything around it. CI starts the stack with a throwaway Google client so those run too.

## Settings

See `.env.example` and `infra/docker-compose.yml` for the full list.

| Setting | Meaning |
|---|---|
| `AUTH_CODE_KEY` | Required. Signs sign-in codes and CSRF tokens. |
| `WEB_ORIGIN` | The one origin allowed to change state. |
| `MAILER` | `log` or `file` in development. Production requires `smtp`. |
| `TRUSTED_PROXIES` | Addresses whose `X-Forwarded-For` the API believes. Set it to the web server's network when the web server sits behind a load balancer. |
| `API_PUBLIC_URL` | Web server only: where it reaches the API's public listener. |
| `WEB_BEHIND_PROXY` | Web server only: `true` only when a trusted proxy in front sets `X-Forwarded-For`. Otherwise the header is dropped and every visitor shares one rate-limit address. |
| `API_ORIGIN_KEY` | Web server only, production only: the key Caddy requires in front of the API. See [deploy/aws.md](deploy/aws.md). |
