#!/usr/bin/env bash
# OEM TLS 门禁演练：已知域名放行、未知域名拒绝、沙箱标记 issued。
# 这不是公网 Let's Encrypt 真签发；Caddy on-demand 仍以 tls-check 为门禁。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

code="$(curl -s -o /dev/null -w '%{http_code}' "$API_URL/v1/public/tls-check?domain=oem.localhost")"
if [[ "$code" != "200" ]]; then
  echo "oem.localhost should be 200, got $code" >&2
  exit 1
fi
code="$(curl -s -o /dev/null -w '%{http_code}' "$API_URL/v1/public/tls-check?domain=evil.example")"
if [[ "$code" != "404" ]]; then
  echo "unknown host should be 404, got $code" >&2
  exit 1
fi

curl -sf -X POST "$API_URL/admin/ops/drills/tls" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' \
  -d '{}' | grep -q '"passed":true'

echo "tls_drill passed (sandbox gate; not public ACME)"
