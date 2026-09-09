#!/usr/bin/env bash
# 模型目录 / 路由组 Sentinel 契约（沙箱 stub，不打真实上游）。
set -euo pipefail
API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"

echo "== no token is 401"
code="$(curl -sS -o /tmp/cat_unauth.json -w '%{http_code}' "$API_URL/v1/models")"
test "$code" = "401"
code="$(curl -sS -o /tmp/cat_unauth_admin.json -w '%{http_code}' "$API_URL/admin/routes")"
test "$code" = "401"

echo "== register stub user"
email="cat-e2e-$RANDOM@example.test"
reg="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email\",\"password\":\"password1\",\"promotion_code\":\"THA1\"}")"
session="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg")"
keyjson="$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"e2e"}')"
key="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" "$keyjson")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' \
  -d '{"code":"THE2E"}' >/dev/null

echo "== no permission is 403"
code="$(curl -sS -o /tmp/cat_user_models.json -w '%{http_code}' -H "Authorization: Bearer $session" "$API_URL/admin/models")"
test "$code" = "403"
code="$(curl -sS -o /tmp/cat_bad_key.json -w '%{http_code}' -H "Authorization: Bearer thk_not_a_real_key" "$API_URL/v1/models")"
test "$code" = "403"
lifejson="$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"lifecycle"}')"
lifekey="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" "$lifejson")"
lifeid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$lifejson")"
curl -sf -X POST "$API_URL/v1/me/api-keys/$lifeid/disable" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{}' >/dev/null
code="$(curl -sS -o /tmp/cat_disabled_key.json -w '%{http_code}' -H "Authorization: Bearer $lifekey" "$API_URL/v1/models")"
test "$code" = "403"

echo "== unknown model / route is 404"
code="$(curl -sS -o /tmp/cat_unknown_model.json -w '%{http_code}' -H "Authorization: Bearer $key" "$API_URL/v1/models/does-not-exist")"
test "$code" = "404"
code="$(curl -sS -o /tmp/cat_unknown_route.json -w '%{http_code}' -X PATCH "$API_URL/admin/routes/rg_does_not_exist" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d '{"strategy":"priority"}')"
test "$code" = "404"

echo "== bad input is 400"
code="$(curl -sS -o /tmp/cat_bad_model.json -w '%{http_code}' -X POST "$API_URL/admin/models" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{}')"
test "$code" = "400"

echo "== success is one attempt (no dual-layer race)"
chat="$(curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"one"}]}')"
rid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['request_id'])" "$chat")"
count="$(curl -sf -H "Authorization: Bearer $key" "$API_URL/v1/requests/$rid/attempts" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['items']))")"
test "$count" = "1"

echo "e2e_catalog_routes ok"
