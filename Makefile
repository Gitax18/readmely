# Readmely task runner. Run `make help` for the list of targets.

# Override for testing, e.g. `make verify-env ENV_FILE=./some.env`. Keep the ./ prefix for relative paths.
ENV_FILE ?= ./.env
COMPOSE := docker compose --env-file $(ENV_FILE) -f infra/docker-compose.yml
GO_DIR  := services/core
AI_DIR  := services/ai
WEB_DIR := apps/web

# Shell prelude for commands that run on the host against the compose stack:
# loads .env and builds the connection URLs the services expect.
LOCAL_ENV = set -a; . $(ENV_FILE); set +a; \
	export DATABASE_URL="postgres://$$POSTGRES_USER:$$POSTGRES_PASSWORD@127.0.0.1:$$POSTGRES_PORT/$$POSTGRES_DB?sslmode=disable"; \
	export REDIS_URL="redis://:$$REDIS_PASSWORD@127.0.0.1:$$REDIS_PORT/0"; \
	export AI_SERVICE_URL="http://127.0.0.1:$${AI_PORT:-8000}"; \
	export MIGRATE_DATABASE_URL="$$DATABASE_URL";

.DEFAULT_GOAL := help
.PHONY: help db-up db-down db-reset db-status db-seed db-new hooks check secrets test-scripts lint-go lint-ai lint-web lint-tools test-go test-ai test-web audit-go audit-ai audit-web env-check up up-app down reset ps logs verify verify-env fmt lint test contracts-check gen gen-check test-e2e test-e2e-stack test-integration audit run-api run-ai run-web docker-build

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-18s %s\n", $$1, $$2}'

env-check:
	@test -f $(ENV_FILE) || { echo "error: $(ENV_FILE) not found. Run: cp .env.example .env  (then change the passwords)"; exit 1; }

# Credentials are pasted into connection URLs, so characters such as @ : / ? # % break
# them in confusing ways (audit A2). Prints variable names only, never values.
URL_SAFE_VARS := POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB REDIS_PASSWORD RUSTFS_ACCESS_KEY RUSTFS_SECRET_KEY AI_SERVICE_TOKEN APP_DB_PASSWORD AI_DB_PASSWORD AUTH_CODE_KEY

hooks: ## Enable the git hooks in .githooks/ (commit message check, staged-file checks)
	@git config core.hooksPath .githooks && echo "ok   git hooks enabled (core.hooksPath=.githooks)"

test-scripts: ## Test the helper scripts in scripts/
	@uv run --frozen --project $(AI_DIR) pytest scripts/tests -q -o addopts='' -p no:cacheprovider -p no:asyncio

verify-env: env-check ## Check that credentials in the env file only use URL-safe characters
	@set -a; . $(ENV_FILE); set +a; bad=""; cr=$$(printf '\r'); \
	for v in $(URL_SAFE_VARS); do \
		eval "val=\$$$$v"; \
		case "$$val" in *[!A-Za-z0-9_-]*) \
			case "$$val" in \
				*"$$cr"*) why="a Windows line ending (CR): save the file with LF line endings";; \
				*[[:space:]]*) why="a space or tab";; \
				*[\"\']*) why="a quote character inside the value";; \
				*[+/=]*) why="+, / or =, typical of base64: generate with openssl rand -hex instead";; \
				*) why="another character that is not a letter, digit, - or _";; \
			esac; \
			echo "     $$v contains $$why"; bad="$$bad $$v";; \
		esac; \
	done; \
	if [ -n "$$bad" ]; then echo "FAIL unsafe characters in:$$bad (use only letters, digits, - and _)"; exit 1; \
	else echo "ok   credentials use URL-safe characters"; fi

up: env-check ## Start the local data stores, wait until they are healthy, and migrate the database
	@$(COMPOSE) up -d --wait
	@$(MAKE) --no-print-directory db-up

up-app: env-check ## Build and start the API, AI and web containers together with the data stores
	@$(MAKE) --no-print-directory up
	@$(COMPOSE) --profile app up -d --build --wait

down: env-check ## Stop the local stack (data is kept)
	@$(COMPOSE) --profile app down

reset: env-check ## Stop the stack and DELETE all local data
	@$(COMPOSE) --profile app down -v

ps: env-check ## Show container status
	@$(COMPOSE) --profile app ps

logs: env-check ## Follow container logs
	@$(COMPOSE) --profile app logs -f

verify: verify-env ## Check credentials, then that the data stores work and are bound to localhost only
	@$(COMPOSE) exec -T postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -tAc "select 1 from pg_available_extensions where name = '"'"'vector'"'"'"' | grep -q 1 \
		&& echo "ok   postgres is up and pgvector is available" \
		|| { echo "FAIL postgres or pgvector"; exit 1; }
	@$(COMPOSE) exec -T redis sh -c 'REDISCLI_AUTH="$$REDIS_PASSWORD" redis-cli ping' | grep -q PONG \
		&& echo "ok   redis is up and requires a password" \
		|| { echo "FAIL redis"; exit 1; }
	@$(COMPOSE) exec -T redis redis-cli ping 2>&1 | grep -q NOAUTH \
		&& echo "ok   redis rejects unauthenticated clients" \
		|| { echo "FAIL redis accepted a client without a password"; exit 1; }
	@$(COMPOSE) exec -T rustfs curl -fsS http://localhost:9000/health > /dev/null \
		&& echo "ok   rustfs is up" \
		|| { echo "FAIL rustfs"; exit 1; }
	@$(COMPOSE) exec -T rustfs curl -s -o /dev/null -w '%{http_code}' http://localhost:9000/ | grep -q 403 \
		&& echo "ok   rustfs rejects unauthenticated requests" \
		|| { echo "FAIL rustfs answered a request without credentials"; exit 1; }
	@if $(COMPOSE) --profile app ps --format '{{.Ports}}' | grep -q '0\.0\.0\.0\|\[::\]'; then \
		echo "FAIL a port is published on all interfaces"; exit 1; \
	else echo "ok   all ports are bound to 127.0.0.1"; fi

fmt: ## Format the Go, Python and web code
	@cd $(GO_DIR) && gofmt -w .
	@cd $(AI_DIR) && uv run --frozen ruff format . && uv run --frozen ruff check --fix .
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm run format

lint: lint-go lint-ai lint-web lint-tools ## Lint the Go, Python and web code (Python and TypeScript include strict type checking)

lint-go:
	@cd $(GO_DIR) && golangci-lint run ./... && golangci-lint run --build-tags=integration ./...

lint-ai:
	@cd $(AI_DIR) && uv run --frozen ruff check . && uv run --frozen ruff format --check . && uv run --frozen mypy

lint-web:
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm run lint && pnpm run format:check

# The helper scripts and the contract tooling share one ruff config.
lint-tools:
	@uv run --frozen --project $(AI_DIR) ruff check --config contracts/ruff.toml contracts scripts && uv run --frozen --project $(AI_DIR) ruff format --check --config contracts/ruff.toml contracts scripts

test: test-go test-ai test-web contracts-check test-scripts ## Run the Go, Python and web unit tests (no containers needed)

test-go:
	@cd $(GO_DIR) && go test -race -count=1 ./...

test-ai:
	@cd $(AI_DIR) && uv run --frozen pytest -q --cov

test-web:
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm test

contracts-check: ## Validate the OpenAPI specs in contracts/ (valid, shared schemas identical, examples match)
	@uv run --frozen --project $(AI_DIR) python contracts/check.py
	@uv run --frozen --project $(AI_DIR) pytest contracts/tests -q -o addopts='' -p no:cacheprovider -p no:asyncio

# Directories that hold only generated code. Never edit them by hand.
GEN_DIRS := services/core/internal/contract services/core/internal/db/store services/ai/app/contract apps/web/src/contract

gen: ## Regenerate the API types (contracts/) and the Go database queries (migrations + SQL); output is committed
	@for pair in core-public:corepublic core-internal:coreinternal ai:ai; do \
		spec=$${pair%%:*}; pkg=$${pair##*:}; \
		mkdir -p $(GO_DIR)/internal/contract/$$pkg; \
		(cd $(GO_DIR) && go tool oapi-codegen -config ../../contracts/gen/$$spec.oapi-codegen.yaml ../../contracts/$$spec.yaml) || exit 1; \
	done
	@cd $(GO_DIR) && CGO_ENABLED=0 go tool sqlc generate
	@mkdir -p $(AI_DIR)/app/contract && touch $(AI_DIR)/app/contract/__init__.py
	@PYTHONWARNINGS='ignore:Black/isort will become optional:FutureWarning' uv run --frozen --project $(AI_DIR) datamodel-codegen --input contracts/ai.yaml --input-file-type openapi \
		--output $(AI_DIR)/app/contract/models.py --output-model-type pydantic_v2.BaseModel \
		--target-python-version 3.13 --formatters black isort --use-annotated --use-standard-collections \
		--use-union-operator --enum-field-as-literal all --disable-timestamp --field-constraints --use-schema-description
	@mkdir -p $(WEB_DIR)/src/contract
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm exec openapi-typescript ../../contracts/core-internal.yaml -o src/contract/core-internal.d.ts > /dev/null
	@cd $(WEB_DIR) && pnpm exec openapi-typescript ../../contracts/core-public.yaml -o src/contract/core-public.d.ts > /dev/null
	@uv run --frozen --project $(AI_DIR) python contracts/extract_examples.py
	@echo "ok   generated types are up to date"

# Fails when generating changes anything: the specs were edited without running
# `make gen`, or a generated file was edited by hand, or one is missing.
gen-check: ## Fail if the generated code is out of date with contracts/
	@b=$$(mktemp); a=$$(mktemp); \
	find $(GEN_DIRS) -type f 2>/dev/null | sort | xargs shasum > $$b 2>/dev/null; \
	$(MAKE) --no-print-directory gen > /dev/null || { rm -f $$b $$a; exit 1; }; \
	find $(GEN_DIRS) -type f | sort | xargs shasum > $$a; \
	if cmp -s $$b $$a; then echo "ok   generated code matches contracts/"; rm -f $$b $$a; \
	else echo "FAIL generated code was out of date: run 'make gen' and commit the result"; \
		diff $$b $$a | grep '^[<>]' | awk '{print "     " $$3}' | sort -u; rm -f $$b $$a; exit 1; fi

test-e2e: ## Run the web browser tests against a production build and a fake API (first time: cd apps/web && pnpm exec playwright install chromium)
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm run test:e2e

test-e2e-stack: env-check ## Browser tests of sign-in against the running stack (first: make up-app; chromium: cd apps/web && pnpm exec playwright install chromium)
	@$(LOCAL_ENV) cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm run test:e2e:stack

# Narrow the Go run while working: make test-integration PKGS=./internal/auth/... (Python is skipped then).
PKGS ?= ./...

test-integration: env-check ## Run the integration tests against the running local stack
	@$(LOCAL_ENV) cd $(GO_DIR) && go test -race -count=1 -tags=integration $(PKGS)
	@if [ "$(PKGS)" = "./..." ]; then $(LOCAL_ENV) cd $(AI_DIR) && uv run --frozen pytest -q -m integration -o addopts=''; fi

GOVULNCHECK_VERSION := v1.10.0

audit: audit-go audit-web audit-ai ## Check Go, Python and web dependencies for known vulnerabilities (needs network)

audit-go:
	@cd $(GO_DIR) && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

audit-web:
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm audit --prod

audit-ai:
	@cd $(AI_DIR) && uv export --frozen --all-groups --no-emit-project --no-hashes --format requirements-txt -q \
		| uv run --frozen pip-audit --strict -r /dev/stdin

# Database migrations run as the owner role. The services never get that connection:
# they log in as readmely_app and readmely_ai, whose passwords `db-up` sets from .env.
db-up: env-check ## Apply database migrations and set the service roles' passwords (needs the stack: make up)
	@$(LOCAL_ENV) cd $(GO_DIR) && go run ./cmd/migrate up && go run ./cmd/migrate roles

db-down: env-check ## Revert the latest migration (development only)
	@$(LOCAL_ENV) cd $(GO_DIR) && APP_ENV="$${APP_ENV:-development}" go run ./cmd/migrate down

db-reset: env-check ## Revert every migration and apply them again (development only; DELETES local data)
	@$(LOCAL_ENV) cd $(GO_DIR) && APP_ENV="$${APP_ENV:-development}" go run ./cmd/migrate reset && go run ./cmd/migrate up && go run ./cmd/migrate roles

db-status: env-check ## List migrations and whether they are applied
	@$(LOCAL_ENV) cd $(GO_DIR) && go run ./cmd/migrate status

db-seed: env-check ## Load fake development data: users, projects, jobs, credits (safe to repeat; development only)
	@$(LOCAL_ENV) cd $(GO_DIR) && APP_ENV="$${APP_ENV:-development}" go run ./cmd/migrate seed

db-new: ## Create an empty migration file: make db-new NAME=add_thing
	@test -n "$(NAME)" || { echo "usage: make db-new NAME=add_thing"; exit 1; }
	@case "$(NAME)" in *[!abcdefghijklmnopqrstuvwxyz0123456789_]*) echo "NAME may only contain lowercase letters, digits and _"; exit 1;; esac
	@f=$(GO_DIR)/migrations/$$(date -u +%Y%m%d%H%M%S)_$(NAME).sql; printf -- '-- +goose Up\n\n-- +goose Down\n' > $$f && echo "created $$f"

# Scans committed history only ("git" mode). It never reads untracked files, so a local .env is never touched.
# SECRETS_LOG_OPTS picks the commits: the whole history of the current branch by default; CI
# narrows it to the commits a pull request adds (BASE..HEAD). Never --all: that includes every
# remote branch, so one branch's mistake would fail every other branch's checks.
SECRETS_LOG_OPTS ?= HEAD

secrets: ## Scan the git history for committed secrets (needs gitleaks)
	@command -v gitleaks > /dev/null || { echo "error: gitleaks is not installed (brew install gitleaks)"; exit 1; }
	@gitleaks git --log-opts="$(SECRETS_LOG_OPTS)" --redact --no-banner --config .gitleaks.toml . && echo "ok   no secrets found in the scanned commits"

check: lint test gen-check audit secrets ## Everything CI runs that needs no containers
	@echo "ok   all checks passed"

run-api: env-check ## Run the API on the host against the local stack (stop the API container first)
	@$(LOCAL_ENV) export DATABASE_URL="postgres://readmely_app:$$APP_DB_PASSWORD@127.0.0.1:$$POSTGRES_PORT/$$POSTGRES_DB?sslmode=disable"; export HTTP_ADDR="127.0.0.1:$${API_PORT:-8080}"; cd $(GO_DIR) && go run ./cmd/api

run-ai: env-check ## Run the AI service on the host against the local stack (bound to 127.0.0.1)
	@$(LOCAL_ENV) export DATABASE_URL="postgresql://readmely_ai:$$AI_DB_PASSWORD@127.0.0.1:$$POSTGRES_PORT/$$POSTGRES_DB?sslmode=disable"; export HTTP_HOST=127.0.0.1 HTTP_PORT="$${AI_PORT:-8000}"; cd $(AI_DIR) && uv run --frozen python -m app.server

run-web: env-check ## Run the web app in dev mode on the host (stop the web container first). Needs the API: `make run-api`
	@$(LOCAL_ENV) export API_INTERNAL_URL="http://127.0.0.1:8081"; cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm exec next dev --hostname 127.0.0.1 --port "$${WEB_PORT:-3000}"

docker-build: ## Build the api, ingest, render, migrate, ai and web images
	@for c in api ingest render migrate; do \
		docker build -q -f infra/docker/core.Dockerfile --build-arg CMD=$$c -t readmely/$$c:dev . || exit 1; \
	done
	@docker build -q -f infra/docker/ai.Dockerfile -t readmely/ai:dev .
	@docker build -q -f infra/docker/web.Dockerfile -t readmely/web:dev .
