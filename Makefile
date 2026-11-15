# Readmely task runner. Run `make help` for the list of targets.

COMPOSE := docker compose --env-file .env -f infra/docker-compose.yml

.DEFAULT_GOAL := help
.PHONY: help env-check up down reset ps logs verify

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

env-check:
	@test -f .env || { echo "error: .env not found. Run: cp .env.example .env  (then change the passwords)"; exit 1; }

up: env-check ## Start the local data stores and wait until they are healthy
	@$(COMPOSE) up -d --wait

down: env-check ## Stop the local stack (data is kept)
	@$(COMPOSE) down

reset: env-check ## Stop the stack and DELETE all local data
	@$(COMPOSE) down -v

ps: env-check ## Show container status
	@$(COMPOSE) ps

logs: env-check ## Follow container logs
	@$(COMPOSE) logs -f

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
	@if $(COMPOSE) ps --format '{{.Ports}}' | grep -q '0\.0\.0\.0\|\[::\]'; then \
		echo "FAIL a port is published on all interfaces"; exit 1; \
	else echo "ok   all ports are bound to 127.0.0.1"; fi
