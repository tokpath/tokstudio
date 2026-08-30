#!/usr/bin/env bash
# M0 端到端验收：健康检查、迁移、RBAC 403、审计写入、Outbox 投递、日志脱敏。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_URL="${TOKENHUB_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${TOKENHUB_BOOTSTRAP_ADMIN_TOKEN:-dev_admin_change_me}"
USER_TOKEN="${TOKENHUB_BOOTSTRAP_USER_TOKEN:-dev_user_change_me}"
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

have_cmd() { command -v "$1" >/dev/null 2>&1; }

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

if ! curl -sf "$API_URL/healthz" >/dev/null 2>&1; then
  if [[ -z "${TOKENHUB_DATABASE_URL:-}" || -z "${TOKENHUB_REDIS_URL:-}" ]]; then
    if [[ -f .env ]]; then
      set -a
      # shellcheck disable=SC1091
      source .env
      set +a
    else
      set -a
      # shellcheck disable=SC1091
      source .env.example
      set +a
    fi
  fi
  echo "starting local api and worker for e2e"
  (cd "$ROOT/backend" && go run ./cmd/api) >"$API_LOG" 2>&1 &
  API_PID=$!
  STARTED_API=1
  (cd "$ROOT/backend" && go run ./cmd/worker) >"$WORKER_LOG" 2>&1 &
  WORKER_PID=$!
  STARTED_WORKER=1
  wait_http "$API_URL/healthz"
fi

echo "== healthz"
health="$(curl -sf "$API_URL/healthz")"
echo "$health"
echo "$health" | grep -q '"status":"ok"'

echo "== readyz"
ready_ok=0
for _ in $(seq 1 20); do
  ready="$(curl -sS "$API_URL/readyz" || true)"
  if echo "$ready" | grep -q '"postgres":"ok"' \
    && echo "$ready" | grep -q '"redis":"ok"' \
    && echo "$ready" | grep -q '"migrations":"ok"' \
    && echo "$ready" | grep -q '"outbox_worker":"ok"'; then
    ready_ok=1
    break
  fi
  sleep 0.5
done
echo "$ready"
if [[ "$ready_ok" != "1" ]]; then
  echo "readyz did not become fully ok" >&2
  exit 1
fi

echo "== metrics"
# 先存再 grep：Prometheus 文本很长，curl|grep -q 会 SIGPIPE（pipefail 退出码 23）
metrics="$(curl -sf "$API_URL/metrics")"
echo "$metrics" | grep -q "go_goroutines"

echo "== unauthenticated admin is 403"
code="$(curl -sS -o /tmp/tokenhub_unauth.json -w "%{http_code}" "$API_URL/admin/audit-logs")"
test "$code" = "403"

echo "== end user cannot read audit"
code="$(curl -sS -o /tmp/tokenhub_user.json -w "%{http_code}" -H "Authorization: Bearer $USER_TOKEN" "$API_URL/admin/audit-logs")"
test "$code" = "403"

echo "== admin writes audit probe"
probe="$(curl -sf -X POST -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/audit-probes")"
echo "$probe"
echo "$probe" | grep -q 'audit.probe'

echo "== admin lists audit"
logs="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/audit-logs")"
echo "$logs" | grep -q 'audit.probe'

echo "== outbox stats after probe"
stats="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/outbox/stats")"
echo "$stats"

echo "== wait worker publish"
published=0
for _ in $(seq 1 20); do
  stats="$(curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" "$API_URL/admin/outbox/stats")"
  if echo "$stats" | grep -Eq '"published":[1-9]'; then
    published=1
    break
  fi
  sleep 0.5
done
if [[ "$published" != "1" ]]; then
  echo "outbox was not published: $stats" >&2
  if [[ -f "$WORKER_LOG" ]]; then
    tail -n 50 "$WORKER_LOG" >&2 || true
  fi
  exit 1
fi

echo "== logs do not contain raw bootstrap tokens"
if [[ -s "$API_LOG" ]]; then
  if grep -F "$ADMIN_TOKEN" "$API_LOG"; then
    echo "admin token leaked into api logs" >&2
    exit 1
  fi
  grep -q '"service":"tokenhub"' "$API_LOG"
fi

echo "M0 e2e passed"
