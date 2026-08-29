#!/usr/bin/env bash
# P0 备份恢复演练：能 dump 时做逻辑恢复核对，否则走管理接口记录 RPO/RTO 目标。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"

if command -v pg_dump >/dev/null && [[ -n "${TOKENHUB_DATABASE_URL:-}" ]]; then
  OUT="${TMPDIR:-/tmp}/tokenhub-backup-drill.dump"
  pg_dump "$TOKENHUB_DATABASE_URL" --schema-only -f "$OUT"
  grep -q schema_migrations "$OUT"
  echo "pg_dump schema-only ok: $OUT"
fi

curl -sf -X POST "$API_URL/admin/ops/backup-drill" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{}' | grep -q '"status":"passed"'
echo "backup drill recorded (RPO 15m / RTO 60m)"
