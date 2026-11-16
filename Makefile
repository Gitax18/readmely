# Readmely task runner. Run `make help` for the list of targets.

COMPOSE := docker compose --env-file .env -f infra/docker-compose.yml
GO_DIR  := services/core

# Shell prelude for commands that run on the host against the compose stack:
# loads .env and builds the connection URLs the Go services expect.
LOCAL_ENV = set -a; . ./.env; set +a; \
	export DATABASE_URL="postgres://$$POSTGRES_USER:$$POSTGRES_PASSWORD@127.0.0.1:$$POSTGRES_PORT/$$POSTGRES_DB?sslmode=disable"; \
	export REDIS_URL="redis://:$$REDIS_PASSWORD@127.0.0.1:$$REDIS_PORT/0";

.DEFAULT_GOAL := help
.PHONY: help env-check up up-app down reset ps logs verify fmt lint test test-integration run-api docker-build

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-18s %s\n", $$1, $$2}'

env-check:
	@test -f .env || { echo "error: .env not found. Run: cp .env.example .env  (then change the passwords)"; exit 1; }

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

verify: env-check ## Check that the data stores work and are bound to localhost only
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

fmt: ## Format the Go code
	@cd $(GO_DIR) && gofmt -w .

lint: ## Lint the Go code
	@cd $(GO_DIR) && golangci-lint run ./... && golangci-lint run --build-tags=integration ./...

test: ## Run the unit tests (no containers needed)
	@cd $(GO_DIR) && go test -race -count=1 ./...

test-integration: env-check ## Run the integration tests against the running local stack
	@$(LOCAL_ENV) cd $(GO_DIR) && go test -race -count=1 -tags=integration ./internal/health/...

run-api: env-check ## Run the API on the host against the local stack (stop the API container first)
	@$(LOCAL_ENV) export HTTP_ADDR="127.0.0.1:$${API_PORT:-8080}"; cd $(GO_DIR) && go run ./cmd/api

docker-build: ## Build the api, ingest and render images
	@for c in api ingest render; do \
		docker build -q -f infra/docker/core.Dockerfile --build-arg CMD=$$c -t readmely/$$c:dev . || exit 1; \
	done
