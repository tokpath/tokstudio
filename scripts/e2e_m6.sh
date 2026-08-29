#!/usr/bin/env bash
# M6 端到端：代理层级归因、佣金冻结/冲正、渠道额度不超发、分销脱敏。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"
AGENT_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}-agent"
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

echo "== register kol2 via promotion"
email="m6-$RANDOM@example.test"
reg="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email\",\"password\":\"password1\",\"promotion_code\":\"THB-KOL2\"}")"
echo "$reg" | grep -q chn_reseller_b
session="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"code":"THE2E"}' >/dev/null
key="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"e2e"}')")"

echo "== usage accrues frozen hierarchy"
chat="$(curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"m6"}]}')"
echo "$chat" | grep -q request_id
usage="$(curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/usage")"
uid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['items'][0]['id'])" "$usage")"
comms="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/commissions?usage_event_id=$uid")"
echo "$comms" | grep -q direct
echo "$comms" | grep -q override
echo "$comms" | grep -q channel
echo "$comms" | grep -q frozen

echo "== agent scope is masked and isolated"
KOL1_TOKEN="${ADMIN_TOKEN}-kol1"
KOL2_TOKEN="${ADMIN_TOKEN}-kol2"
curl -sf -H "Authorization: Bearer $AGENT_TOKEN" "$API_URL/v1/partner/me" | grep -q '"role_type":"agent"'
curl -sf -H "Authorization: Bearer $KOL2_TOKEN" "$API_URL/v1/partner/me" | grep -q '"role_type":"kol_l2"'
agentEmail="m6-agent-$RANDOM@example.test"
curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$agentEmail\",\"password\":\"password1\",\"promotion_code\":\"THB-AGENT\"}" >/dev/null
users="$(curl -sf -H "Authorization: Bearer $AGENT_TOKEN" "$API_URL/v1/partner/users")"
echo "$users" | grep -q '\*\*\*'
echo "$users" | grep -q THB-KOL2
echo "$users" | grep -q THB-AGENT
if echo "$users" | grep -q "$email"; then
  echo "agent saw raw email" >&2
  exit 1
fi
kol2users="$(curl -sf -H "Authorization: Bearer $KOL2_TOKEN" "$API_URL/v1/partner/users")"
echo "$kol2users" | grep -q THB-KOL2
if echo "$kol2users" | grep -q THB-AGENT; then
  echo "kol2 saw agent-attributed users" >&2
  exit 1
fi
kol1users="$(curl -sf -H "Authorization: Bearer $KOL1_TOKEN" "$API_URL/v1/partner/users")"
if echo "$kol1users" | grep -q THB-AGENT; then
  echo "kol1 saw agent-only users" >&2
  exit 1
fi
exportcsv="$(curl -sf -H "Authorization: Bearer $AGENT_TOKEN" "$API_URL/v1/partner/export")"
echo "$exportcsv" | grep -q amount_minor
if echo "$exportcsv" | grep -qi prompt; then
  echo "export leaked prompt" >&2
  exit 1
fi

echo "== refund reverses commission"
rid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['request_id'])" "$chat")"
curl -sf -X POST "$API_URL/admin/refunds" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"request_id\":\"$rid\"}" >/dev/null
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/commissions?usage_event_id=$uid" | grep -q reversed

echo "== unfreeze, monthly settle, manual payout"
email2="m6b-$RANDOM@example.test"
reg2="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email2\",\"password\":\"password1\",\"promotion_code\":\"THB-KOL2\"}")"
s2="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg2")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $s2" -H 'Content-Type: application/json' -d '{"code":"THE2E"}' >/dev/null
k2="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $s2" -H 'Content-Type: application/json' -d '{"name":"e2e2"}')")"
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $k2" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"settle"}]}' >/dev/null
u2="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['items'][0]['id'])" \
  "$(curl -sf -H "Authorization: Bearer $s2" "$API_URL/v1/me/usage")")"
curl -sf -X POST "$API_URL/admin/commissions/unfreeze" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"usage_event_id\":\"$u2\"}" >/dev/null
batch="$(curl -sf -X POST "$API_URL/admin/commissions/settle?ignore_minimum=1" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' -d '{}')"
echo "$batch" | grep -q amount_minor
sid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['items'][0]['id'])" "$batch")"
curl -sf -X POST "$API_URL/admin/settlements/$sid/payout" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d '{"method":"manual","reference":"e2e-wire"}' | grep -q paid

echo "== channel quota cannot over-issue"
q="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channel-quotas/chn_reseller_b")"
avail="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['quota']['available_minor'])" "$q")"
curl -sf -X POST "$API_URL/admin/channel-quotas/grant" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"channel_org_id\":\"chn_reseller_b\",\"amount_minor\":-$avail}" >/dev/null
email3="m6q-$RANDOM@example.test"
reg3="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email3\",\"password\":\"password1\",\"promotion_code\":\"THB-KOL2\"}")"
s3="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg3")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $s3" -H 'Content-Type: application/json' -d '{"code":"THE2E"}' >/dev/null
k3="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $s3" -H 'Content-Type: application/json' -d '{"name":"e2eq"}')")"
code="$(curl -s -o /tmp/m6-quota.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $k3" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"quota"}]}')"
curl -sf -X POST "$API_URL/admin/channel-quotas/grant" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"channel_org_id\":\"chn_reseller_b\",\"amount_minor\":$avail}" >/dev/null
if [[ "$code" != "402" ]]; then
  echo "expected 402 when channel quota is empty, got $code $(cat /tmp/m6-quota.json)" >&2
  exit 1
fi

echo "== commission policy"
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/commission-policy" | grep -q direct_bps
code="$(curl -s -o /tmp/m6-policy.json -w '%{http_code}' -X PATCH "$API_URL/admin/commission-policy" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"direct_bps":1600,"override_bps":500,"channel_bps":500,"team_bps":0,"cap_bps":3500,"freeze_days":7,"min_settle_minor":1000000}')"
if [[ "$code" != "409" ]]; then echo "policy without confirm should 409, got $code $(cat /tmp/m6-policy.json)" >&2; exit 1; fi
curl -sf -X PATCH "$API_URL/admin/commission-policy" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d '{"direct_bps":1500,"override_bps":500,"channel_bps":500,"team_bps":0,"cap_bps":3500,"freeze_days":7,"min_settle_minor":1000000,"version":"m6-v1"}' \
  | grep -q '"direct_bps":1500'

echo "== health"
curl -sf "$API_URL/healthz" | grep -q 0.1.0-m
echo "M6 e2e passed"
