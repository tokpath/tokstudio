#!/usr/bin/env bash
# 恢复演练：解密最近一份加密备份到临时库（或至少验证密文可解开）。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

KEY="${TOKENHUB_BACKUP_KEY:-dev-backup-key-change-me}"
OFFSITE="${TOKENHUB_BACKUP_OFFSITE_DIR:-$ROOT/backups/offsite}"
LATEST="$(ls -1t "$OFFSITE"/tokenhub-*.dump.enc 2>/dev/null | head -1 || true)"
if [[ -z "$LATEST" ]]; then
  echo "no encrypted backup; run scripts/backup_encrypt.sh first" >&2
  exit 1
fi

DEC="$(mktemp)"
trap 'rm -f "$DEC"' EXIT
openssl enc -d -aes-256-cbc -pbkdf2 -pass pass:"$KEY" -in "$LATEST" -out "$DEC"
# custom dump 文件头是 PGDMP
if ! head -c 5 "$DEC" | grep -q PGDMP; then
  echo "decrypted payload is not a pg_dump custom file" >&2
  exit 1
fi
echo "restore drill decrypt ok: $LATEST"
if command -v createdb >/dev/null && command -v pg_restore >/dev/null; then
  DRILL_DB="${TOKENHUB_RESTORE_DRILL_DB:-tokenhub_restore_drill}"
  ADMIN_URL="${TOKENHUB_DATABASE_ADMIN_URL:-postgres://tokenhub:tokenhub@127.0.0.1:5432/postgres?sslmode=disable}"
  dropdb --if-exists --maintenance-db="$ADMIN_URL" "$DRILL_DB" 2>/dev/null || true
  createdb --maintenance-db="$ADMIN_URL" "$DRILL_DB" || true
  TARGET="${TOKENHUB_DATABASE_URL%/*}/$DRILL_DB"
  pg_restore --dbname="$TARGET" --clean --if-exists "$DEC" || true
  echo "optional pg_restore attempted on $DRILL_DB"
fi
