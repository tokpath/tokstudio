#!/usr/bin/env bash
# M6 端到端：归因、两跳佣金冻结/冲正、渠道额度不超发、分销脱敏。
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
q0="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channel-quotas/chn_reseller_b")"
avail0="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['quota']['available_minor'])" "$q0")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"code":"THE2E"}' >/dev/null
q1="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channel-quotas/chn_reseller_b")"
avail1="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['quota']['available_minor'])" "$q1")"
issued="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['quota'].get('issued_minor',0))" "$q1")"
if [[ "$((avail0 - avail1))" -ne 10000000 ]]; then
  echo "redeem should deduct 10 USD from channel available: before=$avail0 after=$avail1" >&2
  exit 1
fi
if [[ "$issued" -lt 10000000 ]]; then
  echo "quota should record issued allocation: $q1" >&2
  exit 1
fi
key="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"e2e"}')")"

echo "== usage accrues frozen direct+indirect"
chat="$(curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Sandbox-Mode: content' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"m6-indirect-xxxxxxxxxxxxxxxxxxxxxxxx"}]}')"
echo "$chat" | grep -q request_id
q2="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channel-quotas/chn_reseller_b")"
avail2="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['quota']['available_minor'])" "$q2")"
if [[ "$avail2" -ne "$avail1" ]]; then
  echo "chat must not deduct channel available again: after_redeem=$avail1 after_chat=$avail2" >&2
  exit 1
fi
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/channel/allocations?channel_id=chn_reseller_b" | grep -q granted_minor
usage="$(curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/usage")"
uid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['items'][0]['id'])" "$usage")"
comms="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/commissions?usage_event_id=$uid")"
echo "$comms" | grep -q '"kind":"direct"'
echo "$comms" | grep -q '"kind":"indirect"'
if echo "$comms" | grep -q '"kind":"override"' || echo "$comms" | grep -q '"kind":"channel"'; then
  echo "old four-bucket kinds must not appear: $comms" >&2
  exit 1
fi
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

echo "== unfreeze does not bypass the freeze period"
email2="m6b-$RANDOM@example.test"
reg2="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email2\",\"password\":\"password1\",\"promotion_code\":\"THB-KOL2\"}")"
s2="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg2")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $s2" -H 'Content-Type: application/json' -d '{"code":"THE2E"}' >/dev/null
k2="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $s2" -H 'Content-Type: application/json' -d '{"name":"e2e2"}')")"
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $k2" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Sandbox-Mode: content' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"settle-xxxxxxxxxxxxxxxxxxxxxxxx"}]}' >/dev/null
u2="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['items'][0]['id'])" \
  "$(curl -sf -H "Authorization: Bearer $s2" "$API_URL/v1/me/usage")")"
unfreeze="$(curl -sf -X POST "$API_URL/admin/commissions/unfreeze" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"usage_event_id\":\"$u2\"}")"
python3 -c 'import json,sys; assert json.loads(sys.argv[1])["unfrozen"] == 0' "$unfreeze"
# Due-date advancement is an internal fixture in TestSettlementJourney, never a public API bypass.
# That integration test covers concurrent settlement, payout retry, and refund reversal.

echo "== channel quota cannot over-issue"
q="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channel-quotas/chn_reseller_b")"
avail="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['quota']['available_minor'])" "$q")"
curl -sf -X POST "$API_URL/admin/channel-quotas/grant" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"channel_org_id\":\"chn_reseller_b\",\"amount_minor\":-$avail}" >/dev/null
email3="m6q-$RANDOM@example.test"
reg3="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email3\",\"password\":\"password1\",\"promotion_code\":\"THB-KOL2\"}")"
s3="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg3")"
code="$(curl -s -o /tmp/m6-quota.json -w '%{http_code}' -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $s3" -H 'Content-Type: application/json' -d '{"code":"THE2E"}')"
curl -sf -X POST "$API_URL/admin/channel-quotas/grant" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"channel_org_id\":\"chn_reseller_b\",\"amount_minor\":$avail}" >/dev/null
if [[ "$code" != "402" ]]; then
  echo "expected 402 when redeeming against empty channel quota, got $code $(cat /tmp/m6-quota.json)" >&2
  exit 1
fi

echo "== commission policy"
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/commission-policy" | grep -q direct_bps
code="$(curl -s -o /tmp/m6-policy.json -w '%{http_code}' -X PATCH "$API_URL/admin/commission-policy" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"direct_bps":1600,"indirect_bps":500,"total_bps":2000,"freeze_days":7,"min_settle_minor":1000000}')"
if [[ "$code" != "409" ]]; then echo "policy without confirm should 409, got $code $(cat /tmp/m6-policy.json)" >&2; exit 1; fi
curl -sf -X PATCH "$API_URL/admin/commission-policy" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d '{"direct_bps":1500,"indirect_bps":500,"total_bps":2000,"freeze_days":7,"min_settle_minor":1000000,"version":"m6-v1"}' \
  | grep -q '"direct_bps":1500'

echo "== D8.2 configurable issue ratio"
CHANNEL_TOKEN="${TOKENHUB_BOOTSTRAP_CHANNEL_TOKEN:-dev_admin_change_me-b}"
stamp="$(date +%s%N)"
ch="$(curl -sf -X POST "$API_URL/admin/channels" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"code\":\"d82-$stamp\",\"type\":\"B\",\"status\":\"active\"}")"
chid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$ch")"
curl -sf -X POST "$API_URL/admin/channel-quotas/grant" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"channel_org_id\":\"$chid\",\"amount_minor\":100000000}" >/dev/null
rule0="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channel-quotas/$chid/issue-rule")"
bps0="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['rule']['issue_ratio_bps'])" "$rule0")"
if [[ "$bps0" != "10000" ]]; then
  echo "default issue ratio must be 10000, got $rule0" >&2
  exit 1
fi
code="$(curl -s -o /tmp/d82-noconfirm.json -w '%{http_code}' -X PATCH "$API_URL/admin/channel-quotas/$chid/issue-rule" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"issue_ratio_bps":12000}')"
if [[ "$code" != "409" ]]; then
  echo "issue-rule without confirm should 409, got $code $(cat /tmp/d82-noconfirm.json)" >&2
  exit 1
fi
code="$(curl -s -o /tmp/d82-channel.json -w '%{http_code}' -X PATCH "$API_URL/admin/channel-quotas/$chid/issue-rule" \
  -H "Authorization: Bearer $CHANNEL_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d '{"issue_ratio_bps":12000}')"
if [[ "$code" != "403" ]]; then
  echo "channel admin cannot patch issue-rule, got $code $(cat /tmp/d82-channel.json)" >&2
  exit 1
fi
curl -sf -X PATCH "$API_URL/admin/channel-quotas/$chid/issue-rule" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d '{"issue_ratio_bps":12000}' | grep -q '"issue_ratio_bps":12000'
promo="THX-D82-$stamp"
curl -sf -X POST "$API_URL/admin/promotion-codes" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"channel_org_id\":\"$chid\",\"code\":\"$promo\"}" >/dev/null
email4="d82-$RANDOM@example.test"
reg4="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email4\",\"password\":\"password1\",\"promotion_code\":\"$promo\"}")"
s4="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg4")"
q3="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channel-quotas/$chid")"
avail3="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['quota']['available_minor'])" "$q3")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $s4" -H 'Content-Type: application/json' -d '{"code":"THE2E"}' >/dev/null
q4="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/channel-quotas/$chid")"
avail4="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['quota']['available_minor'])" "$q4")"
if [[ "$((avail3 - avail4))" -ne 12000000 ]]; then
  echo "1.2x redeem should deduct 12 USD: before=$avail3 after=$avail4 $q4" >&2
  exit 1
fi

echo "== health"
curl -sf "$API_URL/healthz" | grep -q 0.1.0-m
echo "M6 e2e passed"
