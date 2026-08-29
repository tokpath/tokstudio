#!/usr/bin/env bash
# M5 端到端验收：套餐审核、权益 FIFO、支付 webhook 幂等、续费失败、国内支付不伪造自动续费。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"
CHANNEL_TOKEN="${TOKENHUB_BOOTSTRAP_CHANNEL_TOKEN:-dev_admin_change_me-b}"
STARTED_API=0
STARTED_WORKER=0
API_PID=""
WORKER_PID=""
API_LOG="$(mktemp)"
WORKER_LOG="$(mktemp)"

cleanup() {
  if [[ "$STARTED_API" == "1" && -n "$API_PID" ]]; then
    kill "$API_PID" 2>/dev/null || true
  fi
  if [[ "$STARTED_WORKER" == "1" && -n "$WORKER_PID" ]]; then
    kill "$WORKER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  local tries=60
  for _ in $(seq 1 "$tries"); do
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
  elif [[ -f .env.example ]]; then
    set -a
    # shellcheck disable=SC1091
    source .env.example
    set +a
  fi
fi

if ! curl -sf "$API_URL/healthz" >/dev/null 2>&1; then
  echo "starting local api and worker for e2e"
  (cd "$ROOT/backend" && go run ./cmd/api) >"$API_LOG" 2>&1 &
  API_PID=$!
  STARTED_API=1
  (cd "$ROOT/backend" && go run ./cmd/worker) >"$WORKER_LOG" 2>&1 &
  WORKER_PID=$!
  STARTED_WORKER=1
  wait_http "$API_URL/healthz"
fi

SIGN_KEY="${TOKENHUB_PAYMENT_SIGN_KEY:-${TOKENHUB_ENCRYPTION_KEY:-dev-only-32-byte-key-change-me!!}}"

echo "== public plans"
curl -sf "$API_URL/v1/plans" | grep -q pln_echo_month

echo "== register"
email="m5-$RANDOM@example.test"
reg="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email\",\"password\":\"password1\",\"promotion_code\":\"THA1\"}")"
session="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg")"
uid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['user']['id'])" "$reg")"

echo "== channel cheap plan enters review"
cheap="$(curl -sf -X POST "$API_URL/admin/plans" -H "Authorization: Bearer $CHANNEL_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"E2E Cheap","price_minor":1000,"items":[{"unit_type":"usd_credit","included_amount":1}]}')"
echo "$cheap" | grep -q pending_review
pid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$cheap")"
curl -sf -X POST "$API_URL/admin/plans/$pid/review" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"action":"approve","reason":"e2e"}' | grep -q published

echo "== bonus then stripe subscribe"
curl -sf -X POST "$API_URL/admin/entitlements/bonus" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$uid\",\"unit_type\":\"usd_credit\",\"amount\":2000000,\"expires_in_seconds\":3600}" >/dev/null
sub="$(curl -sf -X POST "$API_URL/v1/me/subscriptions" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -d '{"plan_id":"pln_echo_month","adapter":"stripe","payment_method_ref":"pm_ok"}')"
echo "$sub" | grep -q pending
oid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['checkout']['order']['id'])" "$sub")"
sid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['subscription']['id'])" "$sub")"
evt="evt-e2e-m5-$RANDOM"
sig="$(python3 -c "import hmac,hashlib,sys; print(hmac.new(sys.argv[1].encode(), (sys.argv[2]+'|'+sys.argv[3]+'|'+sys.argv[4]).encode(), hashlib.sha256).hexdigest())" "$SIGN_KEY" "$evt" "$oid" "paid")"
curl -sf -X POST "$API_URL/v1/payments/stripe/webhook" -H 'Content-Type: application/json' \
  -H "X-Tokenhub-Payment-Signature: $sig" -d "{\"event_id\":\"$evt\",\"order_id\":\"$oid\",\"status\":\"paid\"}" | grep -q '"duplicate":false\|"item"'
curl -sf -X POST "$API_URL/v1/payments/stripe/webhook" -H 'Content-Type: application/json' \
  -H "X-Tokenhub-Payment-Signature: $sig" -d "{\"event_id\":\"$evt\",\"order_id\":\"$oid\",\"status\":\"paid\"}" | grep -q duplicate
curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/entitlements" | grep -q usd_credit
curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/subscriptions" | grep -q active

echo "== chat covered by entitlements"
keyjson="$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"e2e"}')"
key="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" "$keyjson")"
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"plan-pay"}]}' | grep -q request_id

echo "== alipay does not auto-renew"
ali="$(curl -sf -X POST "$API_URL/v1/me/subscriptions" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -d '{"plan_id":"pln_echo_month","adapter":"alipay"}')"
echo "$ali" | grep -q manual
aoid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['checkout']['order']['id'])" "$ali")"
asid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['subscription']['id'])" "$ali")"
aevt="evt-ali-$RANDOM"
asig="$(python3 -c "import hmac,hashlib,sys; print(hmac.new(sys.argv[1].encode(), (sys.argv[2]+'|'+sys.argv[3]+'|paid').encode(), hashlib.sha256).hexdigest())" "$SIGN_KEY" "$aevt" "$aoid")"
curl -sf -X POST "$API_URL/v1/payments/alipay/webhook" -H 'Content-Type: application/json' \
  -H "X-Tokenhub-Payment-Signature: $asig" -d "{\"event_id\":\"$aevt\",\"order_id\":\"$aoid\",\"status\":\"paid\"}" >/dev/null
curl -sf -X POST "$API_URL/admin/subscriptions/$asid/force-period-end" -H "Authorization: Bearer $ADMIN_TOKEN" | grep -q true
curl -sf -X POST "$API_URL/admin/subscriptions/process-renewals" -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/subscriptions" | grep -q past_due

echo "== stripe renewal failure"
fail="$(curl -sf -X POST "$API_URL/v1/me/subscriptions" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -d '{"plan_id":"pln_echo_month","adapter":"stripe","payment_method_ref":"pm_fail"}')"
foid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['checkout']['order']['id'])" "$fail")"
fsid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['subscription']['id'])" "$fail")"
fevt="evt-fail-$RANDOM"
fsig="$(python3 -c "import hmac,hashlib,sys; print(hmac.new(sys.argv[1].encode(), (sys.argv[2]+'|'+sys.argv[3]+'|paid').encode(), hashlib.sha256).hexdigest())" "$SIGN_KEY" "$fevt" "$foid")"
curl -sf -X POST "$API_URL/v1/payments/stripe/webhook" -H 'Content-Type: application/json' \
  -H "X-Tokenhub-Payment-Signature: $fsig" -d "{\"event_id\":\"$fevt\",\"order_id\":\"$foid\",\"status\":\"paid\"}" >/dev/null
curl -sf -X POST "$API_URL/admin/subscriptions/$fsid/force-period-end" -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
curl -sf -X POST "$API_URL/admin/subscriptions/process-renewals" -H "Authorization: Bearer $ADMIN_TOKEN" >/dev/null
curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/subscriptions" | grep -q past_due

echo "== refund reverses unused entitlements"
curl -sf -X POST "$API_URL/admin/payments/$oid/refund" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' | grep -q refunded

echo "== wechat wallet topup"
wal="$(curl -sf -X POST "$API_URL/v1/payments/orders" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -d '{"adapter":"wechat","amount_minor":1000000,"purpose":"wallet"}')"
woid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['checkout']['order']['id'])" "$wal")"
wevt="evt-wal-$RANDOM"
wsig="$(python3 -c "import hmac,hashlib,sys; print(hmac.new(sys.argv[1].encode(), (sys.argv[2]+'|'+sys.argv[3]+'|paid').encode(), hashlib.sha256).hexdigest())" "$SIGN_KEY" "$wevt" "$woid")"
curl -sf -X POST "$API_URL/v1/payments/wechat/webhook" -H 'Content-Type: application/json' \
  -H "X-Tokenhub-Payment-Signature: $wsig" -d "{\"event_id\":\"$wevt\",\"order_id\":\"$woid\",\"status\":\"paid\"}" >/dev/null
curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/balance" | grep -q available

echo "== health"
curl -sf "$API_URL/healthz" | grep -q 0.1.0-m
curl -sf "$API_URL/readyz" | grep -q ready

echo "M5 e2e passed"
