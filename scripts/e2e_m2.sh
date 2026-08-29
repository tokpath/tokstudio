#!/usr/bin/env bash
set -euo pipefail
API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"

reset_route() {
  curl -sf -X PATCH "$API_URL/admin/providers/prd_echo_primary" -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"health":"available"}' >/dev/null || true
  curl -sf -X PATCH "$API_URL/admin/routes/rg_echo" -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"strategy":"priority"}' >/dev/null || true
}
trap reset_route EXIT

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

echo "== tool calls, json schema, vision, reasoning"
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"lookup"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}' | grep -q tool_calls
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"tool","tool_call_id":"call_echo","content":"sunny"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}' | grep -q 'tool-result:sunny'
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_object"}}' | grep -q 'ok'
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"https://example.test/a.png"}}]}]}' | grep -q vision:describe
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"think"}],"reasoning_effort":"low"}' | grep -q reasoning_tokens
curl -sf -X POST "$API_URL/v1/messages" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"lookup"}],"tools":[{"name":"lookup"}]}' | grep -q tool_use

echo "== stream"
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","stream":true,"messages":[{"role":"user","content":"hi"}]}' | grep -q 'data:'

echo "== OEM docs whitelist"
docs="$(curl -sf -H 'Host: oem.localhost' "$API_URL/v1/public/docs-context")"
echo "$docs" | grep -q tokenhub/oem-demo
echo "$docs" | grep -q Aurora

echo "== provider health"
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/providers" | grep -q echo-primary

echo "== health and price routing strategies"
curl -sf -X PATCH "$API_URL/admin/providers/prd_echo_primary" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"health":"degraded"}' >/dev/null
curl -sf -X PATCH "$API_URL/admin/routes/rg_echo" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"strategy":"health"}' >/dev/null
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"health"}]}' | grep -q echo-backup
curl -sf -X PATCH "$API_URL/admin/providers/prd_echo_primary" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"health":"available"}' >/dev/null
curl -sf -X PATCH "$API_URL/admin/routes/rg_echo" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"strategy":"priority"}' >/dev/null
curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"priority"}]}' | grep -q echo-primary

echo "M2 e2e passed"
