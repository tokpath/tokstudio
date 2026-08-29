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

echo "== admin catalog, gemini, 2fa"
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/providers" | grep -q gemini-flash
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/models" | grep -q google/gemini-flash
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/routes" | grep -q rg_gemini
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"google/gemini-flash","messages":[{"role":"user","content":"gemini"}]}' | grep -q gemini
code="$(curl -s -o /tmp/m7-prov.json -w '%{http_code}' -X POST "$API_URL/admin/providers" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"slug":"no-confirm","name":"x"}')"
if [[ "$code" != "409" ]]; then
  echo "expected 409 creating provider without confirm, got $code" >&2
  exit 1
fi
slug="ops-e2e-$RANDOM"
curl -sf -X POST "$API_URL/admin/providers" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' -d "{\"name\":\"Ops E2E\",\"slug\":\"$slug\",\"adapter\":\"test\"}" | grep -q "$slug"
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/api-keys" | grep -q prefix
setup="$(curl -sf -X POST "$API_URL/admin/me/2fa/setup" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}')"
echo "$setup" | grep -q otpauth
# 不在共享管理员上启用 2FA，避免后续脚本被 totp_required 打断

echo "== api key rotate disable expire and billing export"
life="$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"life"}')"
kid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$life")"
oldk="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" "$life")"
newk="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys/$kid/rotate" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{}')")"
code="$(curl -s -o /tmp/m7-oldk.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $oldk" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"old"}]}')"
if [[ "$code" != "403" ]]; then echo "rotated old key should 403, got $code" >&2; exit 1; fi
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $newk" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"new"}]}' | grep -q request_id
curl -sf -X POST "$API_URL/v1/me/api-keys/$kid/disable" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{}' >/dev/null
code="$(curl -s -o /tmp/m7-dis.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $newk" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"off"}]}')"
if [[ "$code" != "403" ]]; then echo "disabled key should 403, got $code" >&2; exit 1; fi
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/billing/export" | grep -q gross_profit
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/usage?format=csv" | grep -q request_id
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/api-keys?limit=5" | grep -q next_cursor
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/api-keys?format=csv" | grep -q prefix
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/media?format=csv" | grep -q kind
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/models?q=echo" | grep -q tokenhub/echo-1
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/audit-logs?format=csv" | grep -q action
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channels?format=csv" | grep -q official
code="$(curl -s -o /tmp/m7-ssrf.json -w '%{http_code}' -X POST "$API_URL/admin/providers" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"name\":\"ssrf\",\"slug\":\"ssrf-$RANDOM\",\"adapter\":\"openai\",\"base_url\":\"http://169.254.169.254/\"}")"
if [[ "$code" != "400" ]]; then echo "metadata url should 400, got $code $(cat /tmp/m7-ssrf.json)" >&2; exit 1; fi
curl -sf "$API_URL/v1/public/tls-check?domain=oem.localhost" >/dev/null
code="$(curl -s -o /dev/null -w '%{http_code}' "$API_URL/v1/public/tls-check?domain=evil.example")"
if [[ "$code" != "404" ]]; then echo "unknown host should 404, got $code" >&2; exit 1; fi
curl -sf -X POST "$API_URL/admin/brands/brd_oem/tls/issue" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{}' | grep -q issued

echo "== admin role isolation"
FINANCE_TOKEN="${ADMIN_TOKEN}-finance"
OPS_TOKEN="${ADMIN_TOKEN}-ops"
TECH_TOKEN="${ADMIN_TOKEN}-tech"
AUDIT_TOKEN="${ADMIN_TOKEN}-audit"
curl -sf -H "Authorization: Bearer $FINANCE_TOKEN" "$API_URL/admin/me" | grep -q finance_admin
curl -sf -H "Authorization: Bearer $TECH_TOKEN" "$API_URL/admin/me" | grep -q tech_admin
curl -sf -H "Authorization: Bearer $AUDIT_TOKEN" "$API_URL/admin/audit-logs" >/dev/null
curl -sf -H "Authorization: Bearer $FINANCE_TOKEN" "$API_URL/admin/billing/export" | grep -q gross_profit
code="$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $FINANCE_TOKEN" "$API_URL/admin/providers")"
if [[ "$code" != "403" ]]; then echo "finance must not list providers, got $code" >&2; exit 1; fi
code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API_URL/admin/refunds" -H "Authorization: Bearer $AUDIT_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"request_id":"missing"}')"
if [[ "$code" != "403" ]]; then echo "audit must not refund, got $code" >&2; exit 1; fi
code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API_URL/admin/refunds" -H "Authorization: Bearer $OPS_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"request_id":"missing"}')"
if [[ "$code" != "403" ]]; then echo "ops must not refund, got $code" >&2; exit 1; fi
curl -sf -X POST "$API_URL/admin/providers/prd_echo_primary/health-check" -H "Authorization: Bearer $TECH_TOKEN" -H 'Content-Type: application/json' -d '{}' | grep -q health

echo "== chat idempotency 24h"
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: e2e-chat-$email" -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"idem"}]}' >/tmp/m7-idem1.json
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: e2e-chat-$email" -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"idem"}]}' >/tmp/m7-idem2.json
python3 -c "import json; a=json.load(open('/tmp/m7-idem1.json')); b=json.load(open('/tmp/m7-idem2.json')); assert a.get('request_id')==b.get('request_id')"
code="$(curl -s -o /tmp/m7-idem3.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: e2e-chat-$email" -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"other"}]}')"
if [[ "$code" != "409" ]]; then echo "mismatched idempotency should 409, got $code $(cat /tmp/m7-idem3.json)" >&2; exit 1; fi

echo "== session creates media"
curl -sf -X POST "$API_URL/v1/videos" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: e2e-m7-sess" -d '{"model":"bytedance/seedance-1.0","prompt":"console","duration":5}' | grep -q id

echo "== loadtest"
bash "$ROOT/scripts/loadtest_limits.sh"

echo "== health"
curl -sf "$API_URL/healthz" | grep -q 0.1.0-m7
echo "M7 e2e passed"
