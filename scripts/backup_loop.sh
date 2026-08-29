#!/bin/sh
# Compose 里每天做一次加密全量备份，并删除超过 30 天的文件。
set -eu

INTERVAL="${TOKENHUB_BACKUP_INTERVAL_SECONDS:-86400}"
OFFSITE="${TOKENHUB_BACKUP_OFFSITE_DIR:-/backups/offsite}"
KEY="${TOKENHUB_BACKUP_KEY:-dev-backup-key-change-me}"
mkdir -p "$OFFSITE"

if ! command -v openssl >/dev/null 2>&1; then
  apk add --no-cache openssl >/dev/null
fi

while true; do
  STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
  RAW="$(mktemp)"
  pg_dump --format=custom --file="$RAW" "$TOKENHUB_DATABASE_URL"
  openssl enc -aes-256-cbc -pbkdf2 -salt -pass pass:"$KEY" -in "$RAW" -out "$OFFSITE/tokenhub-$STAMP.dump.enc"
  rm -f "$RAW"
  find "$OFFSITE" -name 'tokenhub-*.dump.enc' -mtime +30 -delete 2>/dev/null || true
  echo "encrypted backup: $OFFSITE/tokenhub-$STAMP.dump.enc"
  sleep "$INTERVAL"
done
