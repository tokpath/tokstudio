#!/usr/bin/env bash
set -euo pipefail
API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"

echo "== register"
email="m3-$RANDOM@example.test"
reg="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email\",\"password\":\"password1\",\"promotion_code\":\"THA1\"}")"
session="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg")"
keyjson="$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"e2e"}')"
key="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" "$keyjson")"

echo "== empty wallet returns 402 and no attempts"
code="$(curl -sS -o /tmp/m3_empty.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" \
  -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"no-money"}]}')"
test "$code" = "402"
python3 -c "import json; e=json.load(open('/tmp/m3_empty.json')); assert e['error']['code']=='insufficient_balance'"

echo "== redeem code credits wallet"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -d '{"code":"THE2E"}' | grep -q paid
curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/balance" | grep -q available_minor

echo "== chat settles usage from price snapshot"
chat="$(curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"hello-bill"}]}')"
echo "$chat" | grep -q echo-primary
rid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['request_id'])" "$chat")"
usage="$(curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/usage")"
echo "$usage" | grep -q "$rid"
old="$(python3 -c "import json,sys; items=json.loads(sys.argv[1])['items']; print(items[0]['customer_amount_minor'], items[0]['id'])" "$usage")"
amount="${old%% *}"
usgid="${old##* }"
test "$amount" -gt 0

echo "== replay usage is idempotent"
noconfirm="$(curl -sS -o /tmp/m3_replay409.json -w '%{http_code}' -X POST "$API_URL/admin/usage/replay" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"request_id\":\"$rid\",\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":4}}")"
test "$noconfirm" = "409"
r1="$(curl -sf -X POST "$API_URL/admin/usage/replay" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"request_id\":\"$rid\",\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":4}}")"
r2="$(curl -sf -X POST "$API_URL/admin/usage/replay" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"request_id\":\"$rid\",\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":4}}")"
python3 -c "import json,sys; a=json.loads(sys.argv[1])['item']; b=json.loads(sys.argv[2])['item']; assert a['charge_id']==b['charge_id'] and a['amount_minor']==b['amount_minor']" "$r1" "$r2"

echo "== publish new price does not rewrite old bill"
curl -sf -X POST "$API_URL/admin/price-books" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","input":"0.01","output":"0.02","currency":"USD"}' >/dev/null
usage2="$(curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/usage")"
python3 -c "import json,sys; items=json.loads(sys.argv[1])['items']; assert str(items[0]['customer_amount_minor'])==sys.argv[2]" "$usage2" "$amount"

echo "== missing usage goes pending and does not guess-debit"
omit="$(curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Omit-Usage: 1' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"no-usage"}]}')"
orid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['request_id'])" "$omit")"
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/billing/report" | grep -q pending_reconciliation_count
curl -sf -X POST "$API_URL/admin/usage/replay" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"request_id\":\"$orid\",\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":4}}" | grep -q confirmed

echo "== refund and commission reversal"
bal1="$(curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/balance")"
curl -sf -X POST "$API_URL/admin/refunds" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"request_id\":\"$rid\"}" | grep -q reversed
bal2="$(curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/balance")"
python3 -c "import json,sys; a=json.loads(sys.argv[1])['balance']['available_minor']; b=json.loads(sys.argv[2])['balance']['available_minor']; assert b>a" "$bal1" "$bal2"
curl -sf -X POST "$API_URL/admin/commissions/recalc" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' -H 'Content-Type: application/json' \
  -d "{\"usage_event_id\":\"$usgid\"}" | grep -q policy_version

echo "== manual topup confirm"
top="$(curl -sf -X POST "$API_URL/v1/topups" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -d '{"amount_minor":1000000,"payment_method":"manual"}')"
tid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$top")"
curl -sf -X POST "$API_URL/admin/topups/$tid/confirm" -H "Authorization: Bearer $ADMIN_TOKEN" -H 'X-Tokenhub-Confirm: 1' | grep -q paid

echo "== ledger and report"
curl -sf -H "Authorization: Bearer $session" "$API_URL/v1/me/ledger" | grep -q topup
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/billing/report" | grep -q revenue_minor
curl -sf "$API_URL/healthz" | grep -q tokenhub-api

echo "M3 e2e passed"
