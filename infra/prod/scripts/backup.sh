#!/bin/bash
# Dumps the database to S3 (s3://$OPS_BUCKET/backups/YYYY-MM-DD.dump, custom format, compressed).
# Run daily by readmely-backup.timer; safe to run by hand. The bucket's lifecycle rule deletes
# dumps after 14 days. Restore steps are in docs/deploy/aws.md.
set -euo pipefail

APP_DIR=/opt/readmely
value() { sed -n "s/^$1='\(.*\)'\$/\1/p" "${APP_DIR}/.env"; }
bucket=$(value OPS_BUCKET)
user=$(value POSTGRES_USER)
db=$(value POSTGRES_DB)
key="backups/$(date -u +%F).dump"

"${APP_DIR}/release/scripts/compose.sh" exec -T postgres \
  pg_dump --username "${user}" --dbname "${db}" --format custom --compress 6 \
  | aws s3 cp - "s3://${bucket}/${key}" --sse AES256 --only-show-errors

echo "backup: s3://${bucket}/${key}"
