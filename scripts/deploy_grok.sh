#!/usr/bin/env bash
# Atlas 在宿主机落地 grok 预览：https://grok.tokpath.com
# 只应被 GitHub Actions deploy-grok（feature/grokbot CI 全绿）调用。
# 独立检出 /root/workspace/tokstudio-grok + compose 项目 tokstudio-grok。
# 绝不进入 /root/workspace/tokstudio，也不覆盖 release → test.tokpath.com。
# nova Caddy 只追加 grok.tokpath.com；已有 test.tokpath.com 块原样保留。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TEST_CHECKOUT="${TOKENHUB_TEST_CHECKOUT:-/root/workspace/tokstudio}"
if [[ "$ROOT" == "$TEST_CHECKOUT" ]]; then
  echo "deploy_grok.sh must run from a separate checkout, not ${TEST_CHECKOUT}" >&2
  exit 1
fi

PUBLIC_BASE_URL="${TOKENHUB_PUBLIC_BASE_URL:-https://grok.tokpath.com}"
if [[ "$PUBLIC_BASE_URL" == *"test.tokpath.com"* ]]; then
  echo "refusing to point grok stack at test.tokpath.com" >&2
  exit 1
fi

NOVA_CADDY="${NOVA_CADDY:-/root/workspace/nova/Caddyfile}"
CADDY_SNIPPET="$ROOT/deploy/caddy-grok.tokpath.com.caddy"
ENV_FILE="${TOKENHUB_ENV_FILE:-.env}"
COMPOSE_PROJECT="${COMPOSE_PROJECT_NAME:-tokstudio-grok}"
if [[ "$COMPOSE_PROJECT" == "tokstudio" ]]; then
  echo "refusing compose project name tokstudio (that is the test stack)" >&2
  exit 1
fi

export TOKENHUB_EDGE_HTTP_PORT="${TOKENHUB_EDGE_HTTP_PORT:-9180}"
export TOKENHUB_EDGE_HTTPS_PORT="${TOKENHUB_EDGE_HTTPS_PORT:-9543}"

if [[ "${TOKENHUB_DEPLOY_SKIP_ENV:-}" != "1" ]]; then
  if [[ ! -f "$ENV_FILE" ]]; then
    cp .env.example "$ENV_FILE"
  fi

  export PUBLIC_BASE_URL
  export ENV_FILE
  python3 - <<'PY'
import os
from pathlib import Path

url = os.environ["PUBLIC_BASE_URL"]
path = Path(os.environ["ENV_FILE"])
replacements = {
    "TOKENHUB_PUBLIC_BASE_URL=": f"TOKENHUB_PUBLIC_BASE_URL={url}",
    "TOKENHUB_WEB_ORIGIN=": f"TOKENHUB_WEB_ORIGIN={url}",
}
out = []
seen = set()
for line in path.read_text().splitlines(True):
    written = False
    for prefix, value in replacements.items():
        if line.startswith(prefix):
            out.append(value + "\n")
            seen.add(prefix)
            written = True
            break
    if not written:
        out.append(line)
for prefix, value in replacements.items():
    if prefix not in seen:
        out.append(value + "\n")
path.write_text("".join(out))
PY
fi

if [[ "${TOKENHUB_DEPLOY_SKIP_DOCKER:-}" != "1" ]]; then
  # 释放已无用的构建缓存，避免 40G 盘在镜像构建时写满。
  docker builder prune -f >/dev/null
  bash "$ROOT/scripts/compose_up_with_minio_recovery.sh" -p "$COMPOSE_PROJECT" -f docker-compose.yml -f docker-compose.grok.yml
fi

had_test=0
if [[ -f "$NOVA_CADDY" ]] && grep -qF "test.tokpath.com" "$NOVA_CADDY"; then
  had_test=1
fi

if [[ -f "$CADDY_SNIPPET" && -f "$NOVA_CADDY" ]]; then
  if ! grep -qF "grok.tokpath.com" "$NOVA_CADDY"; then
    printf "\n" >> "$NOVA_CADDY"
    cat "$CADDY_SNIPPET" >> "$NOVA_CADDY"
  fi
  if [[ "$had_test" -eq 1 ]] && ! grep -qF "test.tokpath.com" "$NOVA_CADDY"; then
    echo "refusing: test.tokpath.com disappeared from nova Caddyfile" >&2
    exit 1
  fi
  if grep -vE '^[[:space:]]*#' "$CADDY_SNIPPET" | grep -qF "test.tokpath.com"; then
    echo "refusing: grok caddy snippet must not mention test.tokpath.com" >&2
    exit 1
  fi
  if [[ "${TOKENHUB_DEPLOY_SKIP_DOCKER:-}" != "1" ]]; then
    docker exec caddy caddy validate --config /etc/caddy/Caddyfile
    docker exec caddy caddy reload --config /etc/caddy/Caddyfile
  fi
fi

if [[ "${TOKENHUB_DEPLOY_SKIP_DOCKER:-}" == "1" ]]; then
  echo "grok preview dry-run ok: ${PUBLIC_BASE_URL} project=${COMPOSE_PROJECT}"
  exit 0
fi

echo "waiting for grok TokenHub healthz"
ok=0
for _ in $(seq 1 45); do
  if curl -fsS --max-time 3 -H "Host: grok.tokpath.com" "http://127.0.0.1/healthz" >/dev/null; then
    ok=1
    break
  fi
  sleep 2
done
if [[ "$ok" -ne 1 ]]; then
  echo "healthz did not become ready on grok.tokpath.com" >&2
  docker compose -p "$COMPOSE_PROJECT" -f docker-compose.yml -f docker-compose.grok.yml ps >&2
  docker compose -p "$COMPOSE_PROJECT" -f docker-compose.yml -f docker-compose.grok.yml logs --no-color --tail=80 minio minio-init api web >&2 || true
  exit 1
fi

echo "TokenHub grok preview is up: ${PUBLIC_BASE_URL}"
