#!/usr/bin/env bash
# Called by the exact-SHA test deployment workflow after successful release CI.
# Does not prune global images, modify database generations or delete volumes.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
[[ -f .env ]] || { echo "Prepare deployment .env and new installation credentials first." >&2; exit 1; }
export TOKENHUB_ENV=production
export TOKENHUB_OFFICIAL_HOST=test.tokpath.com
export TOKENHUB_PUBLIC_BASE_URL=https://test.tokpath.com
export TOKENHUB_WEB_ORIGIN=https://test.tokpath.com
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.token.yml)
"${COMPOSE[@]}" config --quiet
if [[ "$(df -Pk "$ROOT" | awk 'NR == 2 { print $4 }')" -lt 3145728 ]]; then
  echo "Less than 3 GiB free; clean unused build artifacts explicitly before deploying." >&2
  exit 1
fi
# Build once. worker/migrate share api's image; include the optional backup
# image now so startup never initiates another build when that profile is on.
"${COMPOSE[@]}" build api web backup
"${COMPOSE[@]}" up -d postgres redis minio minio-init
# Re-run the explicit migrator on every selected build. It refuses a legacy DB
# and verifies immutable migration checksums before API or worker can start.
"${COMPOSE[@]}" run --rm migrate
TOKENHUB_COMPOSE_NO_BUILD=1 bash "$ROOT/scripts/compose_up_with_minio_recovery.sh" -f docker-compose.yml -f docker-compose.token.yml

NOVA_CADDY="${NOVA_CADDY:-/root/workspace/nova/Caddyfile}"
CADDY_SNIPPET="$ROOT/deploy/caddy-test.tokpath.com.caddy"
if [[ -f "$CADDY_SNIPPET" && -f "$NOVA_CADDY" ]]; then
  if ! grep -qF 'test.tokpath.com' "$NOVA_CADDY"; then
    printf '\n' >> "$NOVA_CADDY"
    cat "$CADDY_SNIPPET" >> "$NOVA_CADDY"
  fi
  docker exec caddy caddy validate --config /etc/caddy/Caddyfile
  docker exec caddy caddy reload --config /etc/caddy/Caddyfile
fi
ready=0
for _ in $(seq 1 45); do
  if curl -fsS --max-time 3 -H 'Host: test.tokpath.com' http://127.0.0.1/healthz >/dev/null &&
     curl -fsS --max-time 3 -H 'Host: test.tokpath.com' http://127.0.0.1/readyz >/dev/null; then
    ready=1
    break
  fi
  sleep 2
done
if [[ "$ready" != 1 ]]; then
  echo 'Preview did not become ready; inspect dependency status and durable job failures.' >&2
  "${COMPOSE[@]}" ps >&2
  exit 1
fi
printf 'TokenHub selected revision is running at %s\n' "$TOKENHUB_PUBLIC_BASE_URL"
