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
	export AI_SERVICE_URL="http://127.0.0.1:$${AI_PORT:-8000}";

.DEFAULT_GOAL := help
.PHONY: help env-check up up-app down reset ps logs verify verify-env fmt lint test test-e2e test-integration audit run-api run-ai run-web docker-build

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-18s %s\n", $$1, $$2}'

env-check:
	@test -f $(ENV_FILE) || { echo "error: $(ENV_FILE) not found. Run: cp .env.example .env  (then change the passwords)"; exit 1; }

# Credentials are pasted into connection URLs, so characters such as @ : / ? # % break
# them in confusing ways (audit A2). Prints variable names only, never values.
URL_SAFE_VARS := POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB REDIS_PASSWORD RUSTFS_ACCESS_KEY RUSTFS_SECRET_KEY AI_SERVICE_TOKEN

verify-env: env-check ## Check that credentials in the env file only use URL-safe characters
	@set -a; . $(ENV_FILE); set +a; bad=""; \
	for v in $(URL_SAFE_VARS); do \
		eval "val=\$$$$v"; \
		case "$$val" in *[!A-Za-z0-9_-]*) bad="$$bad $$v";; esac; \
	done; \
	if [ -n "$$bad" ]; then echo "FAIL unsafe characters in:$$bad (use only letters, digits, - and _)"; exit 1; \
	else echo "ok   credentials use URL-safe characters"; fi

up: env-check ## Start the local data stores and wait until they are healthy
	@$(COMPOSE) up -d --wait

up-app: env-check ## Build and start the API container together with the data stores
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

lint: ## Lint the Go, Python and web code (Python and TypeScript include strict type checking)
	@cd $(GO_DIR) && golangci-lint run ./... && golangci-lint run --build-tags=integration ./...
	@cd $(AI_DIR) && uv run --frozen ruff check . && uv run --frozen ruff format --check . && uv run --frozen mypy
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm run lint && pnpm run format:check

test: ## Run the Go, Python and web unit tests (no containers needed)
	@cd $(GO_DIR) && go test -race -count=1 ./...
	@cd $(AI_DIR) && uv run --frozen pytest -q --cov
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm test

test-e2e: ## Run the web browser tests against a production build and a fake API (first time: cd apps/web && pnpm exec playwright install chromium)
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm run test:e2e

test-integration: env-check ## Run the integration tests against the running local stack
	@$(LOCAL_ENV) cd $(GO_DIR) && go test -race -count=1 -tags=integration ./internal/health/...
	@$(LOCAL_ENV) cd $(AI_DIR) && uv run --frozen pytest -q -m integration -o addopts=''

audit: ## Check Go, Python and web dependencies for known vulnerabilities (needs network)
	@cd $(GO_DIR) && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	@cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm audit --prod
	@cd $(AI_DIR) && uv export --frozen --no-dev --no-hashes --format requirements-txt -q \
		| uv run --frozen pip-audit --strict -r /dev/stdin

run-api: env-check ## Run the API on the host against the local stack (stop the API container first)
	@$(LOCAL_ENV) export HTTP_ADDR="127.0.0.1:$${API_PORT:-8080}"; cd $(GO_DIR) && go run ./cmd/api

run-ai: env-check ## Run the AI service on the host against the local stack (bound to 127.0.0.1)
	@$(LOCAL_ENV) export HTTP_HOST=127.0.0.1 HTTP_PORT="$${AI_PORT:-8000}"; cd $(AI_DIR) && uv run --frozen python -m app.server

run-web: env-check ## Run the web app in dev mode on the host (stop the web container first). Needs the API: `make run-api`
	@$(LOCAL_ENV) export API_INTERNAL_URL="http://127.0.0.1:8081"; cd $(WEB_DIR) && pnpm install --frozen-lockfile --silent && pnpm exec next dev --hostname 127.0.0.1 --port "$${WEB_PORT:-3000}"

docker-build: ## Build the api, ingest, render, ai and web images
	@for c in api ingest render; do \
		docker build -q -f infra/docker/core.Dockerfile --build-arg CMD=$$c -t readmely/$$c:dev . || exit 1; \
	done
	@docker build -q -f infra/docker/ai.Dockerfile -t readmely/ai:dev .
	@docker build -q -f infra/docker/web.Dockerfile -t readmely/web:dev .
