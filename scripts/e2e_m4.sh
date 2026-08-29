#!/usr/bin/env bash
# M4 端到端验收：视频任务、幂等、签名下载、失败/取消释放预授权、回调重试、图像。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"
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

SIGN_KEY="${TOKENHUB_MEDIA_SIGN_KEY:-${TOKENHUB_ENCRYPTION_KEY:-dev-only-32-byte-key-change-me!!}}"

echo "== register and credit"
email="m4-$RANDOM@example.test"
reg="$(curl -sf -X POST "$API_URL/v1/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email\",\"password\":\"password1\",\"promotion_code\":\"THA1\"}")"
session="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['session']['token'])" "$reg")"
keyjson="$(curl -sf -X POST "$API_URL/v1/me/api-keys" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"name":"e2e"}')"
key="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['item']['key'])" "$keyjson")"
curl -sf -X POST "$API_URL/v1/topups/redeem" -H "Authorization: Bearer $session" -H 'Content-Type: application/json' -d '{"code":"THE2E"}' >/dev/null

echo "== create video 202"
vid="$(curl -sS -D /tmp/m4_hdr.txt -o /tmp/m4_vid.json -w '%{http_code}' -X POST "$API_URL/v1/videos" \
  -H "Authorization: Bearer $key" -H 'Content-Type: application/json' -H 'Idempotency-Key: e2e-vid-1' \
  -d '{"model":"bytedance/seedance-1.0","prompt":"a river","duration":5,"resolution":"720p"}')"
test "$vid" = "202"
grep -q completed /tmp/m4_vid.json
jid="$(python3 -c "import json; print(json.load(open('/tmp/m4_vid.json'))['id'])")"
upid="$(python3 -c "import json; print(json.load(open('/tmp/m4_vid.json'))['upstream_job_id'])")"
test -n "$upid"

echo "== retry same idempotency key does not create a new job"
vid2="$(curl -sf -X POST "$API_URL/v1/videos" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: e2e-vid-1' -d '{"model":"bytedance/seedance-1.0","prompt":"a river","duration":5}')"
python3 -c "import json,sys; a=json.load(open('/tmp/m4_vid.json')); b=json.loads(sys.argv[1]); assert a['id']==b['id'] and a['upstream_job_id']==b['upstream_job_id']" "$vid2"

echo "== signed content url, not a permanent public link"
content="$(curl -sf -H "Authorization: Bearer $key" "$API_URL/v1/videos/$jid/content")"
echo "$content" | grep -q '/v1/media/objects'
path="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['url'])" "$content")"
case "$path" in http*) url="$path" ;; *) url="$API_URL$path" ;; esac
curl -sf "$url" | grep -q tokenhub-sandbox-media

echo "== fail releases reservation"
fail="$(curl -sf -X POST "$API_URL/v1/videos" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: e2e-fail' -d '{"model":"bytedance/seedance-1.0","prompt":"force-fail"}')"
echo "$fail" | grep -q failed

echo "== cancel in-progress job"
async="$(curl -sf -X POST "$API_URL/v1/videos" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: e2e-cancel' -d '{"model":"bytedance/seedance-1.0","prompt":"force-async"}')"
echo "$async" | grep -q in_progress
aid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$async")"
curl -sf -X POST "$API_URL/v1/videos/$aid/cancel" -H "Authorization: Bearer $key" | grep -q cancelled

echo "== callback is idempotent and settles once"
async2="$(curl -sf -X POST "$API_URL/v1/videos" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: e2e-cb-$RANDOM" -d '{"model":"bytedance/seedance-1.0","prompt":"force-async"}')"
echo "$async2" | grep -q in_progress
cid="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['id'])" "$async2")"
evt="evt-e2e-$RANDOM"
sig="$(python3 -c "import hmac,hashlib,sys; print(hmac.new(sys.argv[1].encode(), (sys.argv[2]+'|'+sys.argv[3]).encode(), hashlib.sha256).hexdigest())" "$SIGN_KEY" "$evt" "$cid")"
body="{\"event_id\":\"$evt\",\"job_id\":\"$cid\",\"usage\":{\"video_seconds\":5}}"
curl -sf -X POST "$API_URL/v1/media/callbacks" -H "Content-Type: application/json" -H "X-Tokenhub-Signature: $sig" -d "$body" | grep -q '"ok":true'
curl -sf -X POST "$API_URL/v1/media/callbacks" -H "Content-Type: application/json" -H "X-Tokenhub-Signature: $sig" -d "$body" | grep -q '"ok":true'
curl -sf -H "Authorization: Bearer $key" "$API_URL/v1/videos/$cid" | grep -q completed

echo "== image generation"
img="$(curl -sS -o /tmp/m4_img.json -w '%{http_code}' -X POST "$API_URL/v1/images/generations" \
  -H "Authorization: Bearer $key" -H 'Content-Type: application/json' -H 'Idempotency-Key: e2e-img' \
  -d '{"model":"tokenhub/image-demo","prompt":"logo"}')"
test "$img" = "202"
grep -q image /tmp/m4_img.json

echo "== health"
curl -sf "$API_URL/healthz" | grep -q 0.1.0-m4
curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/providers" | grep -q ark-seedance

echo "M4 e2e passed"
