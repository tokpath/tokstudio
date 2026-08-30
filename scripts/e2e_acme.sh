#!/usr/bin/env bash
# RFC 8555 ACME 端到端：对 Pebble 真签发。不是公网 Let's Encrypt。
# .localhost 默认仍沙箱；本脚本用 TOKENHUB_ACME_FORCE=1 走完整协议。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_ACME_API_URL:-http://127.0.0.1:18080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"
PEBBLE_DIR="${TOKENHUB_ACME_DIRECTORY:-https://127.0.0.1:14000/dir}"
STARTED_API=0
STARTED_PEBBLE=0
API_PID=""
API_LOG="$(mktemp)"

cleanup() {
  if [[ "$STARTED_API" == "1" && -n "$API_PID" ]]; then
    kill "$API_PID" 2>/dev/null || true
  fi
  if [[ "$STARTED_PEBBLE" == "1" ]]; then
    docker rm -f tokenhub-pebble-e2e >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  local insecure="${2:-}"
  for _ in $(seq 1 60); do
    if [[ "$insecure" == "insecure" ]]; then
      if curl -skf "$url" >/dev/null; then
        return 0
      fi
    elif curl -sf "$url" >/dev/null; then
      return 0
    fi
    sleep 0.5
  done
  echo "timeout waiting for $url" >&2
  return 1
}

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

if [[ -z "${TOKENHUB_DATABASE_URL:-}" || -z "${TOKENHUB_REDIS_URL:-}" ]]; then
  echo "TOKENHUB_DATABASE_URL and TOKENHUB_REDIS_URL are required" >&2
  exit 1
fi

if ! curl -skf "$PEBBLE_DIR" >/dev/null 2>&1; then
  if ! command -v docker >/dev/null; then
    echo "pebble directory $PEBBLE_DIR is down and docker is missing" >&2
    exit 1
  fi
  echo "starting pebble for ACME e2e"
  docker rm -f tokenhub-pebble-e2e >/dev/null 2>&1 || true
  docker run -d --name tokenhub-pebble-e2e \
    -p 14000:14000 -p 15000:15000 \
    -e PEBBLE_VA_ALWAYS_VALID=1 \
    -e PEBBLE_VA_NOSLEEP=1 \
    -e PEBBLE_WFE_NONCEREJECT=0 \
    ghcr.io/letsencrypt/pebble:2.8.0 \
    -config /test/config/pebble-config.json >/dev/null
  STARTED_PEBBLE=1
  wait_http "$PEBBLE_DIR" insecure
fi

if ! curl -sf "$API_URL/healthz" >/dev/null 2>&1; then
  echo "starting api with ACME directory $PEBBLE_DIR"
  (
    cd "$ROOT/backend"
    TOKENHUB_HTTP_ADDR="${TOKENHUB_ACME_HTTP_ADDR:-:18080}" \
    TOKENHUB_PUBLIC_BASE_URL="$API_URL" \
    TOKENHUB_ACME_DIRECTORY="$PEBBLE_DIR" \
    TOKENHUB_ACME_INSECURE_SKIP_VERIFY=true \
    TOKENHUB_ACME_FORCE=true \
    go run ./cmd/api
  ) >"$API_LOG" 2>&1 &
  API_PID=$!
  STARTED_API=1
  if ! wait_http "$API_URL/healthz"; then
    echo "api failed to start; log:" >&2
    tail -n 80 "$API_LOG" >&2
    exit 1
  fi
fi

code="$(curl -s -o /dev/null -w '%{http_code}' "$API_URL/.well-known/acme-challenge/missing")"
if [[ "$code" != "404" ]]; then
  echo "unknown ACME token should 404, got $code" >&2
  exit 1
fi

code="$(curl -s -o /tmp/acme-409.json -w '%{http_code}' -X POST "$API_URL/admin/brands/brd_oem/tls/issue" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}')"
if [[ "$code" != "409" ]]; then
  echo "expected 409 issuing tls without confirm, got $code $(cat /tmp/acme-409.json)" >&2
  exit 1
fi

body="$(curl -sf -X POST "$API_URL/admin/brands/brd_oem/tls/issue" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'X-Tokenhub-Confirm: 1' \
  -d '{}')"
echo "$body" | grep -q '"tls_status":"issued"'
echo "$body" | grep -q '"tls_issuer":"acme"'
echo "$body" | grep -q '14000/dir'

echo "e2e_acme passed (Pebble RFC 8555; not public Let's Encrypt)"
