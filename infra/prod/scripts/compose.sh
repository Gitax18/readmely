#!/bin/bash
# docker compose for the production stack, with the right files. Use it for everyday operations:
#   sudo /opt/readmely/release/scripts/compose.sh ps
#   sudo /opt/readmely/release/scripts/compose.sh logs -f --tail 100 api
set -euo pipefail
APP_DIR=/opt/readmely
exec docker compose \
  --project-directory "${APP_DIR}/release" \
  --env-file "${APP_DIR}/.env" \
  -f "${APP_DIR}/release/docker-compose.prod.yml" \
  "$@"
