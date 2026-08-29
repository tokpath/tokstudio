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
for path in / /docs /app /channel /admin /login; do
  html="$(curl -sf "$WEB_URL$path")"
  echo "$html" | grep -Eq "公共站点|开发者文档|用户控制台|渠道控制台|平台管理|注册 / 登录"
done
oemdocs="$(curl -sf -H "Host: oem.localhost" "$API_URL/v1/public/docs-context")"
echo "$oemdocs" | grep -q "Aurora OEM"
echo "$oemdocs" | grep -q TOKENHUB_API_KEY
echo "$oemdocs" | grep -q /v1/messages
apphtml="$(curl -sf "$WEB_URL/app")"
echo "$apphtml" | grep -q "接入示例"
channelhtml="$(curl -sf "$WEB_URL/channel")"
echo "$channelhtml" | grep -q "本渠道用户"

echo "M1 e2e passed"
