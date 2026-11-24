#!/bin/bash
# Writes /opt/readmely/.env from SSM Parameter Store (/readmely/prod/*), readable by root only.
#   render-env.sh <ecr-registry> <image-tag>
# Every parameter becomes NAME='value'. Single quotes stop docker compose from expanding "$" in a
# value. Values must be one line without a single quote.
set -euo pipefail

registry=${1:?usage: render-env.sh <ecr-registry> <image-tag>}
tag=${2:?usage: render-env.sh <ecr-registry> <image-tag>}
APP_DIR=/opt/readmely
PREFIX=/readmely/prod/

params=$(aws ssm get-parameters-by-path --path "${PREFIX}" --recursive --with-decryption --output json)

bad=$(jq -r '.Parameters[] | select((.Name | split("/") | last | test("^[A-Z][A-Z0-9_]*$") | not)
  or (.Value | test("[\n\r'"'"']"))) | .Name' <<<"${params}")
if [ -n "${bad}" ]; then
  echo "render-env: these parameters have a bad name or a newline or single quote in the value:" >&2
  echo "${bad}" >&2
  exit 1
fi

tmp=$(mktemp "${APP_DIR}/.env.XXXXXX")   # created with mode 600
trap 'rm -f "${tmp}"' EXIT
jq -r '.Parameters[] | "\(.Name | split("/") | last)='"'"'\(.Value)'"'"'"' <<<"${params}" > "${tmp}"
{
  echo "ECR_REGISTRY='${registry}'"
  echo "IMAGE_TAG='${tag}'"
  echo "AWS_REGION='$(aws configure get region)'"
} >> "${tmp}"

# Checks that would otherwise fail later, inside a container, with a less obvious message.
value() { sed -n "s/^$1='\(.*\)'\$/\1/p" "${tmp}"; }
for name in API_HOST WEB_ORIGIN ORIGIN_KEY OPS_BUCKET POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB \
  APP_DB_PASSWORD AI_DB_PASSWORD REDIS_PASSWORD AUTH_CODE_KEY AI_SERVICE_TOKEN; do
  if [ -z "$(value "${name}")" ]; then echo "render-env: ${PREFIX}${name} is missing" >&2; exit 1; fi
done
# Passwords go into connection URLs: letters, digits, - and _ only (as in .env.example).
for name in POSTGRES_PASSWORD APP_DB_PASSWORD AI_DB_PASSWORD REDIS_PASSWORD; do
  if ! [[ "$(value "${name}")" =~ ^[A-Za-z0-9_-]{16,}$ ]]; then
    echo "render-env: ${name} must be at least 16 letters, digits, - or _" >&2; exit 1
  fi
done
# An empty or short key would make Caddy's gate meaningless.
if [ "$(value ORIGIN_KEY | wc -c)" -lt 33 ]; then
  echo "render-env: ORIGIN_KEY must be at least 32 characters (openssl rand -hex 32)" >&2; exit 1
fi

chmod 600 "${tmp}"
mv "${tmp}" "${APP_DIR}/.env"
trap - EXIT
echo "render-env: wrote ${APP_DIR}/.env ($(grep -c . "${APP_DIR}/.env") settings)"
