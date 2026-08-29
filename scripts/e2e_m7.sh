#!/usr/bin/env bash
# M7 端到端：看板维度、限流、熔断、审计检索、备份演练、支付/媒体异常、灰度。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"
STARTED_API=0
API_PID=""
API_LOG="$(mktemp)"

cleanup() {
  if [[ "$STARTED_API" == "1" && -n "$API_PID" ]]; then
    kill "$API_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  for _ in $(seq 1 60); do
    if curl -sf "$url" >/dev/null; then
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for $url" >&2
  return 1
}

if [[ -z "${TOKENHUB_DATABASE_URL:-}" || -z "${TOKENHUB_REDIS_URL:-}" ]]; then
  if [[ -f .env ]]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
  fi
fi

if ! curl -sf "$API_URL/healthz" >/dev/null 2>&1; then
  echo "starting local api for e2e"
  (cd "$ROOT/backend" && go run ./cmd/api) >"$API_LOG" 2>&1 &
  API_PID=$!
  STARTED_API=1
  wait_http "$API_URL/healthz"
fi

echo "== register and chat for metrics"
email="m7-$RANDOM@example.test"
reg="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email\",\"password\":\"password1\",\"promotion_code\":\"THB-KOL2\"}")"
session="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"code":"THE2E"}' >/dev/null
key="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"e2e"}')")"
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"m7"}]}' | grep -q request_id
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/metrics?dimension=model" | grep -q tokenhub/echo-1
dash="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/ops/dashboard")"
echo "$dash" | grep -q gross_profit_minor
echo "$dash" | grep -q success_rate
echo "$dash" | grep -q acr_b_kol2
echo "$dash" | grep -q low_balance_wallets
code="$(curl -s -o /tmp/m7-confirm.json -w '%{http_code}' -X POST "$API_URL/admin/refunds" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"request_id":"missing"}')"
if [[ "$code" != "409" ]]; then
  echo "expected 409 confirm_required, got $code $(cat /tmp/m7-confirm.json)" >&2
  exit 1
fi

echo "== rpm limit 429"
slow="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"slow","rpm_limit":1}')")"
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $slow" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"one"}]}' >/dev/null
code="$(curl -s -o /tmp/m7-rpm.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $slow" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"two"}]}')"
if [[ "$code" != "429" ]]; then
  echo "expected 429, got $code $(cat /tmp/m7-rpm.json)" >&2
  exit 1
fi

echo "== circuit trip falls back"
curl -sf -X POST "$API_URL/admin/ops/circuit/prd_echo_primary" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"action":"trip"}' >/dev/null
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"fb"}]}' | grep -q echo-backup
curl -sf -X POST "$API_URL/admin/ops/circuit/prd_echo_primary" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"action":"reset"}' >/dev/null

echo "== canary header"
curl -sf -X POST "$API_URL/admin/ops/canary" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"provider_slug":"echo-backup","percent":100}' >/dev/null
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H "X-Tokenhub-Canary: 1" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"canary"}]}' | grep -q echo-backup
curl -sf -X POST "$API_URL/admin/ops/canary" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"provider_slug":"echo-backup","percent":0}' >/dev/null

echo "== audit search, backup and chaos drills"
curl -sf -X POST "$API_URL/admin/audit-probes" -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/audit-logs?action=audit.probe" | grep -q audit.probe
curl -sf -X POST "$API_URL/admin/ops/backup-drill" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}' | grep -q '"rpo_minutes":15'
curl -sf -X POST "$API_URL/admin/ops/drills/payment" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}' | grep -q passed
curl -sf -X POST "$API_URL/admin/ops/drills/media" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}' | grep -q passed
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/ops/runbooks" | grep -q pending_reconciliation

echo "== health"
curl -sf "$API_URL/healthz" | grep -q 0.1.0-m7
echo "M7 e2e passed"
