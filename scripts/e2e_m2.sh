#!/usr/bin/env bash
set -euo pipefail
API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"

echo "== register and create API key"
email="m2-$RANDOM@example.test"
reg="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email\",\"password\":\"password1\",\"promotion_code\":\"THA1\"}")"
session="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg")"
keyjson="$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"e2e"}')"
key="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" "$keyjson")"

echo "== M3 topup so M2 chat still has balance"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -d '{"code":"THE2E"}' >/dev/null

echo "== models list"
curl -sf -H "Authorization: Bearer $key" "$API_URL/v1/models" | grep -q tokenhub/echo-1

echo "== chat completions"
chat="$(curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"hello"}]}')"
echo "$chat" | grep -q echo-primary

echo "== fallback on 429"
fb="$(curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Force-Fail: echo-primary' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"hello"}]}')"
echo "$fb" | grep -q echo-backup
rid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['request_id'])" "$fb")"
curl -sf -H "Authorization: Bearer $key" "$API_URL/v1/requests/$rid/attempts" | grep -q failed

echo "== unsupported param"
code="$(curl -sS -o /tmp/m2_bad.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" \
  -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","logit_bias":{"1":1},"messages":[{"role":"user","content":"x"}]}')"
test "$code" = "400"

echo "== stream"
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","stream":true,"messages":[{"role":"user","content":"hi"}]}' | grep -q 'data:'

echo "== OEM docs whitelist"
docs="$(curl -sf -H 'Host: oem.localhost' "$API_URL/v1/public/docs-context")"
echo "$docs" | grep -q tokenhub/oem-demo
echo "$docs" | grep -q Aurora

echo "== provider health"
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/providers" | grep -q echo-primary

echo "M2 e2e passed"
