#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"
CHANNEL_TOKEN="${TOKENHUB_BOOTSTRAP_CHANNEL_TOKEN:-dev_admin_change_me-b}"
WEB_URL="${TOKENHUB_WEB_ORIGIN:-http://127.0.0.1:3000}"

if ! curl -sf "$API_URL/healthz" >/dev/null; then
  echo "API is not running; start api/worker first or run scripts/e2e_m0.sh" >&2
  exit 1
fi

echo "== register two channel users"
alice="alice-$RANDOM@example.test"
bob="bob-$RANDOM@example.test"
reg_a="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$alice\",\"password\":\"password1\",\"promotion_code\":\"THA1\"}")"
reg_b="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$bob\",\"password\":\"password1\",\"promotion_code\":\"THB1\"}")"
echo "$reg_a" | grep -q chn_official_a
echo "$reg_b" | grep -q chn_reseller_b
token_a="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg_a")"

echo "== user cannot switch channel"
code="$(curl -sS -o /tmp/m1_switch.json -w '%{http_code}' -X POST "$API_URL/v1/me/channel/switch" \
  -H "Authorization: Bearer $token_a" -H 'Content-Type: application/json' \
  -d '{"channel_org_id":"chn_reseller_b"}')"
test "$code" = "403"

echo "== unauth admin is 403"
code="$(curl -sS -o /tmp/m1_unauth.json -w '%{http_code}' "$API_URL/admin/users")"
test "$code" = "403"

echo "== channel admin only sees channel B"
users="$(curl -sf -H "Authorization: Bearer $CHANNEL_TOKEN" "$API_URL/channel/users")"
echo "$users" | grep -q "$bob"
if echo "$users" | grep -q "$alice"; then
  echo "channel isolation leaked user A" >&2
  exit 1
fi
promos="$(curl -sf -H "Authorization: Bearer $CHANNEL_TOKEN" "$API_URL/channel/promotion-codes")"
echo "$promos" | grep -q THB1
if echo "$promos" | grep -q THA1; then
  echo "channel promo leaked official code" >&2
  exit 1
fi
plans="$(curl -sf -H "Authorization: Bearer $CHANNEL_TOKEN" "$API_URL/channel/plans")"
echo "$plans" | grep -q items
usage="$(curl -sf -H "Authorization: Bearer $CHANNEL_TOKEN" "$API_URL/channel/usage")"
echo "$usage" | grep -q prompt_tokens
settlements="$(curl -sf -H "Authorization: Bearer $CHANNEL_TOKEN" "$API_URL/channel/settlements")"
echo "$settlements" | grep -q items
attr="$(curl -sf -H "Authorization: Bearer $CHANNEL_TOKEN" "$API_URL/channel/attribution")"
echo "$attr" | grep -q THB1
if echo "$attr" | grep -q THA1; then
  echo "channel attribution leaked official code" >&2
  exit 1
fi

echo "== public models hide providers"
models="$(curl -sf "$API_URL/v1/public/models")"
echo "$models" | grep -q tokenhub/echo-1
if echo "$models" | grep -q echo-primary; then
  echo "public models leaked provider slug" >&2
  exit 1
fi
oemmodels="$(curl -sf "$API_URL/v1/public/models?host=oem.localhost")"
echo "$oemmodels" | grep -q tokenhub/oem-demo

echo "== OEM brand by host"
oem="$(curl -sf "$API_URL/v1/public/brand?host=oem.localhost")"
echo "$oem" | grep -q "Aurora OEM"

echo "== OTP login echo in development"
otp="$(curl -sf -X POST "$API_URL/v1/auth/otp/request" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$alice\",\"purpose\":\"login\"}")"
code="$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('dev_code',''))" "$otp")"
test -n "$code"
curl -sf -X POST "$API_URL/v1/auth/otp/verify" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$alice\",\"code\":\"$code\"}" | grep -q "$alice"

echo "== Google mock keeps promotion"
start="$(curl -sf "$API_URL/v1/auth/google/start?promotion_code=THC1")"
state="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['state'])" "$start")"
google="$(curl -sf -X POST "$API_URL/v1/auth/google/callback" -H 'Content-Type: application/json' \
  -d "{\"state\":\"$state\",\"code\":\"mock:oem-$RANDOM@example.test\"}")"
echo "$google" | grep -q chn_oem_c

echo "== user settings profile and password"
me="$(curl -sf -H "Authorization: Bearer $token_a" "$API_URL/v1/me")"
echo "$me" | grep -q '"locale":"zh"'
patched="$(curl -sf -X PATCH "$API_URL/v1/me" -H "Authorization: Bearer $token_a" -H 'Content-Type: application/json' \
  -d '{"display_name":"Alice E2E","locale":"en"}')"
echo "$patched" | grep -q 'Alice E2E'
echo "$patched" | grep -q '"locale":"en"'
echo "$patched" | grep -q chn_official_a
curl -sf -X POST "$API_URL/v1/me/password" -H "Authorization: Bearer $token_a" -H 'Content-Type: application/json' \
  -d '{"current_password":"password1","new_password":"password2"}' | grep -q '"ok":true'
old="$(curl -sS -o /tmp/m1_oldpw.json -w '%{http_code}' -X POST "$API_URL/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$alice\",\"password\":\"password1\"}")"
test "$old" = "403"
curl -sf -X POST "$API_URL/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$alice\",\"password\":\"password2\"}" | grep -q "$alice"

echo "== four portals render"
homehtml="$(curl -sf "$WEB_URL/")"
echo "$homehtml" | grep -q "可用模型"
echo "$homehtml" | grep -q "套餐与订阅"
echo "$homehtml" | grep -q "充值"
echo "$homehtml" | grep -q "去登录"
for path in / /docs /app /channel /partner /admin /login; do
  html="$(curl -sf "$WEB_URL$path")"
  echo "$html" | grep -Eq "公共站点|开发者文档|用户控制台|渠道控制台|分销控制台|平台管理|注册 / 登录"
done
oemdocs="$(curl -sf -H "Host: oem.localhost" "$API_URL/v1/public/docs-context")"
echo "$oemdocs" | grep -q "Aurora OEM"
echo "$oemdocs" | grep -q TOKENHUB_API_KEY
echo "$oemdocs" | grep -q /v1/messages
apphtml="$(curl -sf "$WEB_URL/app")"
echo "$apphtml" | grep -q "接入示例"
channelhtml="$(curl -sf "$WEB_URL/channel")"
echo "$channelhtml" | grep -q "本渠道用户"
echo "$channelhtml" | grep -q "本渠道套餐"
echo "$channelhtml" | grep -q "推广链接"
echo "$channelhtml" | grep -q "本渠道用量"
echo "$channelhtml" | grep -q "本渠道归因"
echo "$channelhtml" | grep -q "本渠道结算"
partnerhtml="$(curl -sf "$WEB_URL/partner")"
echo "$partnerhtml" | grep -q "我的层级"
echo "$partnerhtml" | grep -q "范围内用户"
adminhtml="$(curl -sf "$WEB_URL/admin")"
echo "$adminhtml" | grep -q "套餐审核"
echo "$adminhtml" | grep -q "佣金策略"
echo "$adminhtml" | grep -q "支付"
planhtml="$(curl -sf "$WEB_URL/admin/plans")"
echo "$planhtml" | grep -q "套餐审核"
echo "$planhtml" | grep -q "待审核"
usershtml="$(curl -sf "$WEB_URL/admin/users")"
echo "$usershtml" | grep -q "封禁"
echo "$usershtml" | grep -q "改归因"
billhtml="$(curl -sf "$WEB_URL/admin/billing")"
echo "$billhtml" | grep -q "赠送额度"
echo "$billhtml" | grep -q "退消费账单"
chanhtml="$(curl -sf "$WEB_URL/admin/channels")"
echo "$chanhtml" | grep -q "调整额度"
promoshtml="$(curl -sf "$WEB_URL/admin/promos")"
echo "$promoshtml" | grep -q "创建推广角色"
echo "$promoshtml" | grep -q "创建推广码"
commhtml="$(curl -sf "$WEB_URL/admin/commission")"
echo "$commhtml" | grep -q "手工结算"
echo "$commhtml" | grep -q "人工打款"
alertshtml="$(curl -sf "$WEB_URL/admin/alerts")"
echo "$alertshtml" | grep -q "评估告警"
runbookshtml="$(curl -sf "$WEB_URL/admin/runbooks")"
echo "$runbookshtml" | grep -q "应急手册"
settingshtml="$(curl -sf "$WEB_URL/admin/settings")"
echo "$settingshtml" | grep -q "运维开关"
echo "$settingshtml" | grep -q "健康探测"
usagehtml="$(curl -sf "$WEB_URL/admin/usage")"
echo "$usagehtml" | grep -q "回放 usage"

echo "== admin can create acquisition role and promo with confirm"
role409="$(curl -sS -o /tmp/m1_role409.json -w '%{http_code}' -X POST "$API_URL/admin/acquisition-roles" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"channel_org_id":"chn_reseller_b","type":"kol_l2","parent_id":"acr_b_kol1"}')"
test "$role409" = "409"
role="$(curl -sf -X POST "$API_URL/admin/acquisition-roles" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d '{"channel_org_id":"chn_reseller_b","type":"kol_l2","parent_id":"acr_b_kol1"}')"
role_id="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['id'])" "$role")"
promo_code="THB-E2E-$RANDOM"
promo409="$(curl -sS -o /tmp/m1_promo409.json -w '%{http_code}' -X POST "$API_URL/admin/promotion-codes" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"channel_org_id\":\"chn_reseller_b\",\"acquisition_role_id\":\"$role_id\",\"code\":\"$promo_code\"}")"
test "$promo409" = "409"
curl -sf -X POST "$API_URL/admin/promotion-codes" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d "{\"channel_org_id\":\"chn_reseller_b\",\"acquisition_role_id\":\"$role_id\",\"code\":\"$promo_code\"}" | grep -q "$promo_code"

echo "== admin ban blocks login and api key"
carol="carol-$RANDOM@example.test"
reg_c="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$carol\",\"password\":\"password1\",\"promotion_code\":\"THA1\"}")"
token_c="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg_c")"
uid_c="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['user']['id'])" "$reg_c")"
keyjson="$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $token_c" -H 'Content-Type: application/json' -d '{"name":"ban"}')"
key_c="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" "$keyjson")"
noconfirm="$(curl -sS -o /tmp/m1_ban409.json -w '%{http_code}' -X POST "$API_URL/admin/users/$uid_c/ban" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{"reason":"abuse"}')"
test "$noconfirm" = "409"
curl -sf -X POST "$API_URL/admin/users/$uid_c/ban" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"reason":"abuse"}' | grep -q banned
banned_login="$(curl -sS -o /tmp/m1_banned.json -w '%{http_code}' -X POST "$API_URL/v1/auth/login" \
  -H 'Content-Type: application/json' -d "{\"email\":\"$carol\",\"password\":\"password1\"}")"
test "$banned_login" = "403"
banned_me="$(curl -sS -o /tmp/m1_banned_me.json -w '%{http_code}' -H "Authorization: Bearer $token_c" "$API_URL/v1/me")"
test "$banned_me" = "403"
banned_key="$(curl -sS -o /tmp/m1_banned_key.json -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" \
  -H "Authorization: Bearer $key_c" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"no"}]}')"
test "$banned_key" = "403"
audit="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/audit-logs?action=identity.user.ban")"
echo "$audit" | grep -q identity.user.ban
curl -sf -X POST "$API_URL/admin/users/$uid_c/unban" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' -d '{"reason":"appeal"}' | grep -q active
curl -sf -X POST "$API_URL/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$carol\",\"password\":\"password1\"}" | grep -q "$carol"
curl -sf -X POST "$API_URL/admin/users/$uid_c/attribution" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -H 'X-Tokenhub-Confirm: 1' \
  -d '{"promotion_code":"THB1","reason":"manual move"}' | grep -q updated
moved="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/users")"
echo "$moved" | grep -q "$carol"
echo "$moved" | grep -q THB1

echo "M1 e2e passed"
