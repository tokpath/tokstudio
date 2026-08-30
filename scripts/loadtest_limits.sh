#!/usr/bin/env bash
# 限流 / 熔断压测：对 RPM=2 的 Key 连打，必须出现 429；熔断后请求仍成功并落到 backup。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"

email="load-$RANDOM@example.test"
reg="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email\",\"password\":\"password1\",\"promotion_code\":\"THA1\"}")"
session="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"code":"THE2E"}' >/dev/null
key="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"load","rpm_limit":2}')")"
# 熔断验证用另一把不限流的 Key，避免刚打完 429 后同一把 Key 仍被 RPM 挡住。
normal="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" \
  "$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"circuit"}')")"

tmpdir="$(mktemp -d)"
for i in $(seq 1 16); do
  (
    code="$(curl -s -o "$tmpdir/$i.body" -w '%{http_code}' -X POST "$API_URL/v1/chat/completions" \
      -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
      -d "{\"model\":\"tokenhub/echo-1\",\"messages\":[{\"role\":\"user\",\"content\":\"burst-$i\"}]}")"
    echo "$code" >"$tmpdir/$i.code"
  ) &
done
wait
ok=0
limited=0
codes=""
for i in $(seq 1 16); do
  c="$(cat "$tmpdir/$i.code")"
  codes="$codes $c"
  if [[ "$c" == "200" ]]; then ok=$((ok+1)); fi
  if [[ "$c" == "429" ]]; then limited=$((limited+1)); fi
done
echo "rpm burst codes:$codes ok=$ok limited=$limited"
if [[ "$limited" -lt 1 ]]; then
  echo "expected at least one 429 under rpm_limit=2" >&2
  exit 1
fi

curl -sf -X POST "$API_URL/admin/ops/circuit/prd_echo_primary" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -d '{"action":"trip"}' >/dev/null
fb="$(curl -sf -X POST "$API_URL/v1/chat/completions" -H "Authorization: Bearer $normal" -H 'Content-Type: application/json' \
  -d '{"model":"tokenhub/echo-1","messages":[{"role":"user","content":"after-trip"}]}')"
echo "$fb" | grep -q echo-backup
curl -sf -X POST "$API_URL/admin/ops/circuit/prd_echo_primary" -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' -d '{"action":"reset"}' >/dev/null
echo "circuit breaker load path used echo-backup"
echo "loadtest_limits passed"
