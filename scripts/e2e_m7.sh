#!/usr/bin/env bash
# M7 端到端：看板维度、限流、熔断、审计检索、备份演练、支付/媒体异常、灰度。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"
WEB_URL="${TOKENHUB_WEB_ORIGIN:-http://127.0.0.1:3000}"
STARTED_API=0
API_PID=""
API_LOG="$(mktemp)"

cleanup() {
  if [[ "$STARTED_API" == "1" && -n "$API_PID" ]]; then
    kill "$API_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# 先完整下载再检查：大 CSV/导出 + grep -q 会 SIGPIPE（pipefail 退出码 23）
curl_has() {
  local needle="$1"
  shift
  local body
  body="$(curl -sf "$@")" || {
    echo "curl failed: $*" >&2
    return 1
  }
  if ! grep -q -- "$needle" <<<"$body"; then
    echo "missing '$needle' in: $*" >&2
    return 1
  fi
}


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
curl_has request_id -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"m7"}]}'
curl_has tokenhub/echo-1 -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/metrics?dimension=model"
dash="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/ops/dashboard")"
echo "$dash" | grep -q gross_profit_minor
echo "$dash" | grep -q success_rate
echo "$dash" | grep -q acr_b_kol2
echo "$dash" | grep -q low_balance_wallets
echo "$dash" | grep -q latency_p99_ms
echo "$dash" | grep -q http_429
echo "$dash" | grep -q preauth_failed
echo "$dash" | grep -q callback_latency_p95_ms
echo "$dash" | grep -q timeouts
echo "$dash" | grep -q error_codes
echo "$dash" | grep -q prompt_tokens
echo "$dash" | grep -q video_seconds
echo "$dash" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d['dashboard']['thresholds']['min_requests']>=1"
curl_has '"min_requests":10' -X PATCH "$API_URL/admin/ops/thresholds" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d '{"success_rate_min":0.8,"min_requests":10,"pending_count":2}'
curl -sf -X PATCH "$API_URL/admin/ops/thresholds" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d '{"success_rate_min":0.5,"min_requests":5,"pending_count":1}' >/dev/null
series="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/metrics/series?days=7")"
echo "$series" | python3 -c "import json,sys,datetime; d=json.load(sys.stdin); items=d['items']; assert len(items)==7; today=datetime.datetime.utcnow().strftime('%Y-%m-%d'); assert any(i['day']==today and i['requests']>0 for i in items)"
curl_has gross_profit_minor -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/metrics/daily?format=csv&days=7"
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
curl_has echo-backup -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"fb"}]}'
curl -sf -X POST "$API_URL/admin/ops/circuit/prd_echo_primary" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"action":"reset"}' >/dev/null

echo "== canary header"
curl -sf -X POST "$API_URL/admin/ops/canary" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"provider_slug":"echo-backup","percent":100}' >/dev/null
curl_has echo-backup -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H "X-Tokenhub-Canary: 1" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"canary"}]}'
curl -sf -X POST "$API_URL/admin/ops/canary" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"provider_slug":"echo-backup","percent":0}' >/dev/null

echo "== audit search, backup and chaos drills"
curl -sf -X POST "$API_URL/admin/audit-probes" -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
curl_has audit.probe -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/audit-logs?action=audit.probe"
curl_has '"rpo_minutes":15' -X POST "$API_URL/admin/ops/backup-drill" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}'
curl_has passed -X POST "$API_URL/admin/ops/drills/payment" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}'
curl_has passed -X POST "$API_URL/admin/ops/drills/media" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}'
curl_has passed -X POST "$API_URL/admin/ops/drills/tls" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}'
curl_has pending_reconciliation -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/ops/runbooks"
curl_has tls_chaos -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/ops/runbooks"
alertshtml="$(curl -sf "$WEB_URL/admin/alerts")"
echo "$alertshtml" | grep -q "评估告警"
runbookshtml="$(curl -sf "$WEB_URL/admin/runbooks")"
echo "$runbookshtml" | grep -q "应急手册"
settingshtml="$(curl -sf "$WEB_URL/admin/settings")"
echo "$settingshtml" | grep -q "备份演练"
echo "$settingshtml" | grep -q "OEM 证书"
echo "$settingshtml" | grep -q "异常演练"
echo "$settingshtml" | grep -q "支付演练"
echo "$settingshtml" | grep -q "TLS 演练"
routeshtml="$(curl -sf "$WEB_URL/admin/routes")"
echo "$routeshtml" | grep -q "创建路由"
echo "$routeshtml" | grep -q "保存策略"
commhtml="$(curl -sf "$WEB_URL/admin/commission")"
echo "$commhtml" | grep -q "佣金重算"
echo "$commhtml" | grep -q "重算佣金"

echo "== admin catalog, gemini, 2fa"
provhtml="$(curl -sf "$WEB_URL/admin/providers")"
echo "$provhtml" | grep -q "凭据轮换"
echo "$provhtml" | grep -q "轮换凭据"
echo "$provhtml" | grep -q "账号池"
echo "$provhtml" | grep -q "读取账号"
modelhtml="$(curl -sf "$WEB_URL/admin/models")"
echo "$modelhtml" | grep -q "挂载 Provider"
echo "$modelhtml" | grep -q "弃用模型"
curl_has gemini-flash -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/providers"
curl_has google/gemini-flash -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/models"
curl_has rg_gemini -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/routes"
curl_has gemini -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"google/gemini-flash","messages":[{"role":"user","content":"gemini"}]}'
code="$(curl -s -o /tmp/m7-recalc409.json -w '%{http_code}' -X POST "$API_URL/admin/commissions/recalc" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"usage_event_id":"usg_missing"}')"
if [[ "$code" != "409" ]]; then
  echo "expected 409 recalc without confirm, got $code" >&2
  exit 1
fi
ROUTE_MODEL="tokenhub/ops-route-$RANDOM"
curl -sf -X POST "$API_URL/admin/models" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' -d "{\"public_id\":\"$ROUTE_MODEL\",\"vendor\":\"tokenhub\",\"display_name\":\"Ops Route\",\"status\":\"draft\"}" >/dev/null
code="$(curl -s -o /tmp/m7-route409.json -w '%{http_code}' -X POST "$API_URL/admin/routes" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"public_model_id\":\"$ROUTE_MODEL\",\"strategy\":\"priority\"}")"
if [[ "$code" != "409" ]]; then
  echo "expected 409 creating route without confirm, got $code" >&2
  exit 1
fi
ROUTE_JSON="$(curl -sf -X POST "$API_URL/admin/routes" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"public_model_id\":\"$ROUTE_MODEL\",\"strategy\":\"priority\",\"status\":\"active\",\"candidates\":[{\"provider_id\":\"prd_echo_primary\",\"priority\":1,\"weight\":1}]}")"
ROUTE_ID="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$ROUTE_JSON")"
code="$(curl -s -o /tmp/m7-routepatch409.json -w '%{http_code}' -X PATCH "$API_URL/admin/routes/$ROUTE_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"strategy":"health"}')"
if [[ "$code" != "409" ]]; then
  echo "expected 409 patching route without confirm, got $code" >&2
  exit 1
fi
curl_has health -X PATCH "$API_URL/admin/routes/$ROUTE_ID" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"strategy":"health"}'
code="$(curl -s -o /tmp/m7-prov.json -w '%{http_code}' -X POST "$API_URL/admin/providers" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"slug":"no-confirm","name":"x"}')"
if [[ "$code" != "409" ]]; then
  echo "expected 409 creating provider without confirm, got $code" >&2
  exit 1
fi
slug="ops-e2e-$RANDOM"
PROV_JSON="$(curl -sf -X POST "$API_URL/admin/providers" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' -d "{\"name\":\"Ops E2E\",\"slug\":\"$slug\",\"adapter\":\"test\"}")"
echo "$PROV_JSON" | grep -q "$slug"
PROV_ID="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$PROV_JSON")"
code="$(curl -s -o /tmp/m7-cred409.json -w '%{http_code}' -X POST "$API_URL/admin/providers/$PROV_ID/credentials" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"secret":"sk-no-confirm"}')"
if [[ "$code" != "409" ]]; then
  echo "expected 409 rotating credential without confirm, got $code" >&2
  exit 1
fi
CRED_JSON="$(curl -sf -X POST "$API_URL/admin/providers/$PROV_ID/credentials" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"secret":"sk-e2e-rotate-never-echo"}')"
echo "$CRED_JSON" | grep -q credential_ref
if echo "$CRED_JSON" | grep -q sk-e2e-rotate-never-echo; then
  echo "rotate must not echo plaintext secret" >&2
  exit 1
fi
curl_has provider.credential.rotate -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/audit-logs?action=provider.credential.rotate"
curl_has prefix -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/api-keys"
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
curl_has request_id -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $newk" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"new"}]}'
curl -sf -X POST "$API_URL/v1/me/api-keys/$kid/disable" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{}' >/dev/null
code="$(curl -s -o /tmp/m7-dis.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $newk" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"off"}]}')"
if [[ "$code" != "403" ]]; then echo "disabled key should 403, got $code" >&2; exit 1; fi
curl_has gross_profit -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/billing/export"
curl_has request_id -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/usage?format=csv"
curl_has next_cursor -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/api-keys?limit=5"
curl_has prefix -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/api-keys?format=csv"
curl_has kind -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/media?format=csv"
curl_has tokenhub/echo-1 -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/models?q=echo"
curl_has action -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/audit-logs?format=csv"
curl_has official -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channels?format=csv"
curl_has pln_echo_month -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/plans"
curl_has tokenhub/echo-1 -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/price-books"
curl_has direct_bps -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/commission-policy"
curl_has adapter -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/payments?format=csv"
code="$(curl -s -o /tmp/m7-ssrf.json -w '%{http_code}' -X POST "$API_URL/admin/providers" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"name\":\"ssrf\",\"slug\":\"ssrf-$RANDOM\",\"adapter\":\"openai\",\"base_url\":\"http://169.254.169.254/\"}")"
if [[ "$code" != "400" ]]; then echo "metadata url should 400, got $code $(cat /tmp/m7-ssrf.json)" >&2; exit 1; fi
curl -sf "$API_URL/v1/public/tls-check?domain=oem.localhost" >/dev/null
code="$(curl -s -o /dev/null -w '%{http_code}' "$API_URL/v1/public/tls-check?domain=evil.example")"
if [[ "$code" != "404" ]]; then echo "unknown host should 404, got $code" >&2; exit 1; fi
code="$(curl -s -o /tmp/m7-tls409.json -w '%{http_code}' -X POST "$API_URL/admin/brands/brd_oem/tls/issue" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}')"
if [[ "$code" != "409" ]]; then echo "expected 409 issuing tls without confirm, got $code" >&2; exit 1; fi
curl_has issued -X POST "$API_URL/admin/brands/brd_oem/tls/issue" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{}'

echo "== admin role isolation"
FINANCE_TOKEN="${ADMIN_TOKEN}-finance"
OPS_TOKEN="${ADMIN_TOKEN}-ops"
TECH_TOKEN="${ADMIN_TOKEN}-tech"
AUDIT_TOKEN="${ADMIN_TOKEN}-audit"
curl_has finance_admin -H "Authorization: Bearer $FINANCE_TOKEN" "$API_URL/admin/me"
curl_has tech_admin -H "Authorization: Bearer $TECH_TOKEN" "$API_URL/admin/me"
curl -sf -H "Authorization: Bearer $AUDIT_TOKEN" "$API_URL/admin/audit-logs" >/dev/null
curl_has gross_profit -H "Authorization: Bearer $FINANCE_TOKEN" "$API_URL/admin/billing/export"
code="$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $FINANCE_TOKEN" "$API_URL/admin/providers")"
if [[ "$code" != "403" ]]; then echo "finance must not list providers, got $code" >&2; exit 1; fi
code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API_URL/admin/refunds" -H "Authorization: Bearer $AUDIT_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"request_id":"missing"}')"
if [[ "$code" != "403" ]]; then echo "audit must not refund, got $code" >&2; exit 1; fi
code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API_URL/admin/refunds" -H "Authorization: Bearer $OPS_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"request_id":"missing"}')"
if [[ "$code" != "403" ]]; then echo "ops must not refund, got $code" >&2; exit 1; fi
curl_has health -X POST "$API_URL/admin/providers/prd_echo_primary/health-check" -H "Authorization: Bearer $TECH_TOKEN" -H 'Content-Type: application/json' -d '{}'

echo "== provider account pool"
POOL_SLUG="pool-e2e-$RANDOM"
POOL_JSON="$(curl -sf -X POST "$API_URL/admin/providers" -H "Authorization: Bearer $TECH_TOKEN" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' -d "{\"name\":\"Pool E2E\",\"slug\":\"$POOL_SLUG\",\"adapter\":\"test\"}")"
POOL_ID="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$POOL_JSON")"
curl -sf -X POST "$API_URL/admin/models/attach" -H "Authorization: Bearer $TECH_TOKEN" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' -d "{\"public_id\":\"tokenhub/echo-1\",\"provider_id\":\"$POOL_ID\",\"upstream_model_id\":\"echo-upstream\"}" >/dev/null
COOL_JSON="$(curl -sf -X POST "$API_URL/admin/providers/$POOL_ID/accounts" -H "Authorization: Bearer $TECH_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"secret":"sk-cool","label":"cooling"}')"
echo "$COOL_JSON" | grep -q fingerprint
echo "$COOL_JSON" | grep -qv ciphertext
COOL_ID="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$COOL_JSON")"
curl -sf -X PATCH "$API_URL/admin/providers/$POOL_ID/accounts/$COOL_ID" -H "Authorization: Bearer $TECH_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"cooldown_seconds":120}' >/dev/null
code="$(curl -s -o /tmp/m7-pool-cool.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions?provider.only=$POOL_SLUG" \
  -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"cool"}]}')"
if [[ "$code" != "503" ]]; then echo "cooldown-only pool should 503, got $code $(cat /tmp/m7-pool-cool.json)" >&2; exit 1; fi
curl -sf -X POST "$API_URL/admin/providers/$POOL_ID/accounts" -H "Authorization: Bearer $TECH_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"secret":"sk-hot","label":"hot"}' >/dev/null
curl_has "$POOL_SLUG" -X POST "$API_URL/v1/chat/completions?provider.only=$POOL_SLUG" -H "Authorization: Bearer $key" \
  -H 'Content-Type: application/json' -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"hot"}]}'
code="$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $FINANCE_TOKEN" "$API_URL/admin/providers/$POOL_ID/accounts")"
if [[ "$code" != "403" ]]; then echo "finance must not list accounts, got $code" >&2; exit 1; fi

echo "== model sync draft review publish"
SYNC="$(curl -sf -X POST "$API_URL/admin/providers/$POOL_ID/sync" -H "Authorization: Bearer $OPS_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{}')"
echo "$SYNC" | grep -q '"status":"draft"'
SYNC_ID="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['items'][0]['id'])" "$SYNC")"
python3 -c "import json,sys; ids=[i.get('id') for i in json.load(sys.stdin).get('data',[])]; assert sys.argv[1] not in ids" \
  "$SYNC_ID" <<<"$(curl -sf -H "Authorization: Bearer $key" "$API_URL/v1/models")"
curl_has reviewed -X POST "$API_URL/admin/models/review" -H "Authorization: Bearer $OPS_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"public_id\":\"$SYNC_ID\",\"action\":\"approve\"}"
curl_has published -X POST "$API_URL/admin/models/publish" -H "Authorization: Bearer $OPS_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"public_id\":\"$SYNC_ID\"}"
curl_has "$SYNC_ID" -H "Authorization: Bearer $key" "$API_URL/v1/models"
curl_has deprecated -X POST "$API_URL/admin/models/deprecate" -H "Authorization: Bearer $OPS_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"public_id\":\"$SYNC_ID\"}"
python3 -c "import json,sys; ids=[i.get('id') for i in json.load(sys.stdin).get('data',[])]; assert sys.argv[1] not in ids" \
  "$SYNC_ID" <<<"$(curl -sf -H "Authorization: Bearer $key" "$API_URL/v1/models")"
curl_has "$SYNC_ID" -H "Authorization: Bearer $OPS_TOKEN" "$API_URL/admin/models?q=sync-"

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
curl_has id -X POST "$API_URL/v1/videos" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: e2e-m7-sess" -d '{"model":"bytedance/seedance-1.0","prompt":"console","duration":5}'

echo "== loadtest"
bash "$ROOT/scripts/loadtest_limits.sh"

echo "== health"
curl_has 0.1.0-m7 "$API_URL/healthz"
echo "M7 e2e passed"
