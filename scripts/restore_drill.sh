#!/usr/bin/env bash
# Restore an encrypted backup into a newly created, isolated drill database.
# Never drop an existing database or report a failed restore as successful.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
ENV_FILE="${TOKENHUB_ENV_FILE:-$ROOT/.env}"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  source "$ENV_FILE"
  set +a
fi
for tool in openssl createdb pg_restore psql; do
  command -v "$tool" >/dev/null || { echo "required tool missing: $tool" >&2; exit 1; }
done
KEY="${TOKENHUB_BACKUP_KEY:?TOKENHUB_BACKUP_KEY required}"
OFFSITE="${TOKENHUB_BACKUP_OFFSITE_DIR:-$ROOT/backups/offsite}"
LATEST="${TOKENHUB_RESTORE_BACKUP_FILE:-}"
if [[ -z "$LATEST" ]]; then
  LATEST="$(ls -1t "$OFFSITE"/tokenhub-*.dump.enc 2>/dev/null | head -1 || true)"
fi
[[ -f "$LATEST" ]] || { echo "no encrypted backup found" >&2; exit 1; }
ADMIN_URL="${TOKENHUB_DATABASE_ADMIN_URL:?TOKENHUB_DATABASE_ADMIN_URL required}"
case "$ADMIN_URL" in postgres://*|postgresql://*) ;; *) echo "admin URL must be a PostgreSQL URI" >&2; exit 1;; esac
DRILL_DB="${TOKENHUB_RESTORE_DRILL_DB:-tokenhub_restore_drill_$(date -u +%Y%m%d%H%M%S)_$$}"
[[ "$DRILL_DB" =~ ^tokenhub_restore_drill_[a-z0-9_]+$ && ${#DRILL_DB} -le 63 ]] || {
  echo "drill database must have tokenhub_restore_drill_ prefix and at most 63 safe characters" >&2; exit 1;
}
BASE="${ADMIN_URL%%\?*}"
TARGET="${BASE%/*}/$DRILL_DB"
if [[ "$ADMIN_URL" == *\?* ]]; then TARGET="$TARGET?${ADMIN_URL#*\?}"; fi
DEC="$(mktemp)"
trap 'rm -f "$DEC"' EXIT
# Pass the key through the environment rather than process command arguments.
export TOKENHUB_DRILL_DECRYPT_KEY="$KEY"
openssl enc -d -aes-256-cbc -pbkdf2 -pass env:TOKENHUB_DRILL_DECRYPT_KEY -in "$LATEST" -out "$DEC"
unset TOKENHUB_DRILL_DECRYPT_KEY
pg_restore --list "$DEC" >/dev/null
# createdb must fail if the target already exists; never clean/drop it implicitly.
createdb --maintenance-db="$ADMIN_URL" "$DRILL_DB"
if ! pg_restore --exit-on-error --single-transaction --dbname="$TARGET" "$DEC"; then
  echo "restore FAILED; isolated database retained for inspection: $DRILL_DB" >&2
  exit 1
fi
psql "$TARGET" -v ON_ERROR_STOP=1 -Atc 'SELECT count(*) FROM schema_migrations' >/dev/null
TABLES="$(psql "$TARGET" -v ON_ERROR_STOP=1 -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")"
[[ "$TABLES" -gt 0 ]] || { echo "restore FAILED: no application tables" >&2; exit 1; }
echo "restore passed: $DRILL_DB ($TABLES public tables); retained for data verification"
echo "This verifies this backup only; it does not prove PITR, offsite durability, or production RPO/RTO."
