#!/usr/bin/env bash
# 预览栈共用：docker compose up，若 minio-init 因卷内旧凭证失败则重建 MinIO 卷并重试一次。
# 用法：compose_up_with_minio_recovery.sh <compose 额外参数...>
# 例：compose_up_with_minio_recovery.sh -f docker-compose.yml -f docker-compose.token.yml
#     compose_up_with_minio_recovery.sh -p tokstudio-grok -f docker-compose.yml -f docker-compose.grok.yml
set -euo pipefail

if [[ "$#" -lt 1 ]]; then
  echo "usage: $0 <docker compose args...>" >&2
  exit 2
fi

COMPOSE=(docker compose "$@")

dump_minio_logs() {
  echo "---- minio / minio-init logs ----" >&2
  "${COMPOSE[@]}" logs --no-color --tail=120 minio minio-init >&2 || true
  echo "---- end minio logs ----" >&2
}

project_name() {
  # compose config 的 name；失败时回退目录名（test 栈默认）。
  local name
  name="$("${COMPOSE[@]}" config --format json 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin).get("name") or "")' 2>/dev/null || true)"
  if [[ -z "$name" ]]; then
    name="$(basename "$(pwd)")"
  fi
  printf '%s' "$name"
}

recreate_minio_volume() {
  local project vol
  project="$(project_name)"
  vol="${project}_tokenhub_minio"
  echo "preview recovery: recreating MinIO volume ${vol} (root credentials are frozen in /data)" >&2
  "${COMPOSE[@]}" stop minio minio-init >/dev/null 2>&1 || true
  "${COMPOSE[@]}" rm -f minio minio-init >/dev/null 2>&1 || true
  docker volume rm "$vol" >/dev/null 2>&1 || true
}

if "${COMPOSE[@]}" up --build -d; then
  exit 0
fi

echo "compose up failed; checking MinIO init" >&2
dump_minio_logs
recreate_minio_volume

if "${COMPOSE[@]}" up --build -d; then
  echo "compose up succeeded after MinIO volume recreate" >&2
  exit 0
fi

echo "compose up still failing after MinIO volume recreate" >&2
dump_minio_logs
"${COMPOSE[@]}" ps >&2 || true
exit 1
