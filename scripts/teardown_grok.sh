#!/usr/bin/env bash
# 拆除 grok.tokpath.com 预览栈：停 compose、删 nova Caddy 站点块、可选清理检出。
# 不触碰 test.tokpath.com / /root/workspace/tokstudio。
# 由 GitHub Actions teardown-grok 或人工在宿主机执行；幂等可重复跑。
set -euo pipefail

GROK_CHECKOUT="${TOKENHUB_GROK_CHECKOUT:-/root/workspace/tokstudio-grok}"
TEST_CHECKOUT="${TOKENHUB_TEST_CHECKOUT:-/root/workspace/tokstudio}"
NOVA_CADDY="${NOVA_CADDY:-/root/workspace/nova/Caddyfile}"
COMPOSE_PROJECT="${COMPOSE_PROJECT_NAME:-tokstudio-grok}"
REMOVE_CHECKOUT="${TOKENHUB_GROK_REMOVE_CHECKOUT:-1}"

if [[ "$GROK_CHECKOUT" == "$TEST_CHECKOUT" ]]; then
  echo "refusing: grok checkout must not equal test checkout" >&2
  exit 1
fi

echo "tearing down grok preview (project=${COMPOSE_PROJECT})"

have_docker=0
if command -v docker >/dev/null 2>&1; then
  have_docker=1
fi

if [[ "$have_docker" -eq 1 ]] && docker compose -p "$COMPOSE_PROJECT" ps -q 2>/dev/null | grep -q .; then
  if [[ -f "$GROK_CHECKOUT/docker-compose.yml" ]]; then
    (
      cd "$GROK_CHECKOUT"
      if [[ -f docker-compose.grok.yml ]]; then
        docker compose -p "$COMPOSE_PROJECT" -f docker-compose.yml -f docker-compose.grok.yml down --remove-orphans || true
      else
        docker compose -p "$COMPOSE_PROJECT" down --remove-orphans || true
      fi
    )
  else
    docker compose -p "$COMPOSE_PROJECT" down --remove-orphans || true
  fi
else
  echo "no running containers for project ${COMPOSE_PROJECT} (or docker unavailable)"
fi

# 再扫一遍可能残留的同名容器。
if [[ "$have_docker" -eq 1 ]] && docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qE '^tokstudio-grok'; then
  docker ps -a --format '{{.Names}}' | grep -E '^tokstudio-grok' | while read -r name; do
    docker rm -f "$name" >/dev/null 2>&1 || true
  done
fi

if [[ -f "$NOVA_CADDY" ]]; then
  if grep -qF 'grok.tokpath.com' "$NOVA_CADDY"; then
    python3 - <<'PY' "$NOVA_CADDY"
import pathlib
import re
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
# 去掉 grok.tokpath.com { ... } 整块（含前导空行），保留其余站点。
pattern = re.compile(
    r"\n*grok\.tokpath\.com\s*\{(?:[^{}]|\{[^{}]*\})*\}\n*",
    re.MULTILINE,
)
new_text, n = pattern.subn("\n", text, count=1)
if n == 0 and "grok.tokpath.com" in text:
    # 回退：按行删到匹配的闭合大括号。
    lines = text.splitlines(True)
    out = []
    skipping = False
    depth = 0
    for line in lines:
        if not skipping and "grok.tokpath.com" in line and "{" in line:
            skipping = True
            depth = line.count("{") - line.count("}")
            continue
        if skipping:
            depth += line.count("{") - line.count("}")
            if depth <= 0:
                skipping = False
            continue
        out.append(line)
    new_text = "".join(out)
path.write_text(new_text.rstrip() + "\n")
print(f"removed grok.tokpath.com site block from {path}")
PY
    if grep -qF 'test.tokpath.com' "$NOVA_CADDY"; then
      echo "test.tokpath.com still present in nova Caddyfile"
    else
      echo "warning: test.tokpath.com missing after edit; review ${NOVA_CADDY}" >&2
    fi
    if grep -qF 'grok.tokpath.com' "$NOVA_CADDY"; then
      echo "refusing: grok.tokpath.com still in Caddyfile after teardown" >&2
      exit 1
    fi
    if [[ "$have_docker" -eq 1 ]] && docker ps --format '{{.Names}}' 2>/dev/null | grep -qx caddy; then
      docker exec caddy caddy validate --config /etc/caddy/Caddyfile
      docker exec caddy caddy reload --config /etc/caddy/Caddyfile
    else
      echo "caddy container not running; skipped reload"
    fi
  else
    echo "nova Caddyfile has no grok.tokpath.com block"
  fi
else
  echo "nova Caddyfile not found at ${NOVA_CADDY}; skipped"
fi

if [[ "$REMOVE_CHECKOUT" == "1" && -d "$GROK_CHECKOUT" ]]; then
  if [[ "$GROK_CHECKOUT" == "/" || "$GROK_CHECKOUT" == "/root" || "$GROK_CHECKOUT" == "/root/workspace" ]]; then
    echo "refusing to remove unsafe path ${GROK_CHECKOUT}" >&2
    exit 1
  fi
  rm -rf "$GROK_CHECKOUT"
  echo "removed checkout ${GROK_CHECKOUT}"
fi

echo "grok.tokpath.com teardown complete"
