#!/usr/bin/env bash
# 在 token 机器上拉起 TokenHub 预览：https://test.tokpath.com
# Atlas 编排：只应被 GitHub Actions deploy-token 在合入 release/v0.1.0 后调用。
# 检出固定为 /root/workspace/tokstudio。
# 不抢 80/443；gashub Caddy 继续服务 www.tokpath.com，并加一条 test 主机名反代。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PUBLIC_BASE_URL="${TOKENHUB_PUBLIC_BASE_URL:-https://test.tokpath.com}"
NOVA_CADDY="${NOVA_CADDY:-/root/workspace/nova/Caddyfile}"
CADDY_SNIPPET="$ROOT/deploy/caddy-test.tokpath.com.caddy"

if [[ ! -f .env ]]; then
  cp .env.example .env
fi

export PUBLIC_BASE_URL
python3 - <<'PY'
import os
from pathlib import Path

url = os.environ["PUBLIC_BASE_URL"]
path = Path(".env")
replacements = {
    "TOKENHUB_PUBLIC_BASE_URL=": f"TOKENHUB_PUBLIC_BASE_URL={url}",
    "TOKENHUB_WEB_ORIGIN=": f"TOKENHUB_WEB_ORIGIN={url}",
}
google_keys = {
    "TOKENHUB_GOOGLE_CLIENT_ID",
    "TOKENHUB_GOOGLE_CLIENT_SECRET",
    "TOKENHUB_GOOGLE_REDIRECT_URL",
}

def strip_wrap(value: str) -> tuple[str, bool]:
    v = value.strip()
    quoted = len(v) >= 2 and ((v[0] == v[-1] == '"') or (v[0] == v[-1] == "'"))
    if quoted:
        v = v[1:-1].strip()
    return v, quoted

out = []
seen = set()
google_shape = {}
for line in path.read_text().splitlines(True):
    raw = line.rstrip("\n")
    stripped = raw.strip()
    if stripped and not stripped.startswith("#") and "=" in stripped:
        key, _, value = stripped.partition("=")
        key = key.strip()
        if key in google_keys:
            cleaned, quoted = strip_wrap(value)
            google_shape[key] = {
                "len": len(cleaned),
                "quoted": quoted,
                "looks_web": cleaned.startswith("GOCSPX-"),
                "has_space": any(ch.isspace() for ch in cleaned),
                "suffix": cleaned[-40:] if key.endswith("CLIENT_ID") else "",
                "value": cleaned if key.endswith("REDIRECT_URL") else "",
            }
            out.append(f"{key}={cleaned}\n")
            continue
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

print("=== google oauth .env shape (secrets redacted) ===")
for key in sorted(google_keys):
    info = google_shape.get(key)
    if info is None:
        print(f"{key}=MISSING")
        continue
    if key.endswith("SECRET"):
        print(
            f"{key}=set len={info['len']} looks_web={info['looks_web']} "
            f"had_wrapping_quotes={info['quoted']} has_space={info['has_space']}"
        )
    elif key.endswith("CLIENT_ID"):
        print(
            f"{key}=set len={info['len']} had_wrapping_quotes={info['quoted']} "
            f"suffix={info['suffix']}"
        )
    else:
        print(f"{key}={info['value']!r} had_wrapping_quotes={info['quoted']}")
PY

# 释放已无用的构建缓存，避免 40G 盘在镜像构建时写满。
docker builder prune -f >/dev/null

bash "$ROOT/scripts/compose_up_with_minio_recovery.sh" -f docker-compose.yml -f docker-compose.token.yml

if [[ -f "$CADDY_SNIPPET" && -f "$NOVA_CADDY" ]]; then
  if ! grep -qF "test.tokpath.com" "$NOVA_CADDY"; then
    printf "\n" >> "$NOVA_CADDY"
    cat "$CADDY_SNIPPET" >> "$NOVA_CADDY"
  fi
  docker exec caddy caddy validate --config /etc/caddy/Caddyfile
  docker exec caddy caddy reload --config /etc/caddy/Caddyfile
fi

echo "waiting for TokenHub healthz"
ok=0
for _ in $(seq 1 45); do
  if curl -fsS --max-time 3 -H "Host: test.tokpath.com" "http://127.0.0.1/healthz" >/dev/null; then
    ok=1
    break
  fi
  sleep 2
done
if [[ "$ok" -ne 1 ]]; then
  echo "healthz did not become ready on test.tokpath.com" >&2
  docker compose -f docker-compose.yml -f docker-compose.token.yml ps >&2
  docker compose -f docker-compose.yml -f docker-compose.token.yml logs --no-color --tail=80 minio minio-init api web >&2 || true
  exit 1
fi

echo "TokenHub preview is up: ${PUBLIC_BASE_URL}"
