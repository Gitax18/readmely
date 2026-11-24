#!/bin/bash
# Deploys one release on the instance. The deploy workflow runs it as root through SSM after
# unpacking infra/prod/ into /opt/readmely/release:
#   /opt/readmely/release/scripts/deploy.sh <ecr-registry> <image-tag>
# Rolling back is the same command with an older tag (ECR keeps the last 5 images).
set -euo pipefail

registry=${1:?usage: deploy.sh <ecr-registry> <image-tag>}
tag=${2:?usage: deploy.sh <ecr-registry> <image-tag>}
APP_DIR=/opt/readmely
SCRIPTS="${APP_DIR}/release/scripts"
compose() { "${SCRIPTS}/compose.sh" "$@"; }

echo "==> settings"
"${SCRIPTS}/render-env.sh" "${registry}" "${tag}"
api_host=$(sed -n "s/^API_HOST='\(.*\)'\$/\1/p" "${APP_DIR}/.env")

echo "==> images ${tag}"
aws ecr get-login-password | docker login --username AWS --password-stdin "${registry}"
compose --profile migrate pull --quiet

echo "==> database"
compose up -d --wait postgres redis
compose --profile migrate run --rm migrate up
compose --profile migrate run --rm migrate roles

echo "==> services"
compose up -d --wait --remove-orphans

echo "==> public health check (https://${api_host}/healthz)"
# The very first start includes getting the certificate, which can take a minute.
ok=""
for _ in $(seq 1 30); do
  if curl -fsS --max-time 5 --resolve "${api_host}:443:127.0.0.1" "https://${api_host}/healthz" >/dev/null; then
    ok=1; break
  fi
  sleep 5
done
if [ -z "${ok}" ]; then
  echo "deploy: https://${api_host}/healthz did not answer. Recent Caddy and API logs:" >&2
  compose logs --tail 40 caddy api >&2
  exit 1
fi

echo "==> daily backup timer"
install -m 644 "${APP_DIR}/release/systemd/readmely-backup.service" /etc/systemd/system/
install -m 644 "${APP_DIR}/release/systemd/readmely-backup.timer" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now readmely-backup.timer

echo "==> cleanup"
docker image prune -af --filter "until=168h" >/dev/null
echo "deployed ${tag}"
