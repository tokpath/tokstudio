#!/usr/bin/env bash
# 加密逻辑备份：pg_dump custom format + AES-256，默认写到异地目录 backups/offsite。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

if [[ -z "${TOKENHUB_DATABASE_URL:-}" ]]; then
  echo "TOKENHUB_DATABASE_URL required" >&2
  exit 1
fi

KEY="${TOKENHUB_BACKUP_KEY:-dev-backup-key-change-me}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OFFSITE="${TOKENHUB_BACKUP_OFFSITE_DIR:-$ROOT/backups/offsite}"
mkdir -p "$OFFSITE"
RAW="$(mktemp)"
trap 'rm -f "$RAW"' EXIT

pg_dump --format=custom --file="$RAW" "$TOKENHUB_DATABASE_URL"
openssl enc -aes-256-cbc -pbkdf2 -salt -pass pass:"$KEY" -in "$RAW" -out "$OFFSITE/tokenhub-$STAMP.dump.enc"
# 保留至少 30 天：删除更早的加密备份
find "$OFFSITE" -name 'tokenhub-*.dump.enc' -mtime +30 -delete || true
echo "encrypted backup: $OFFSITE/tokenhub-$STAMP.dump.enc"
