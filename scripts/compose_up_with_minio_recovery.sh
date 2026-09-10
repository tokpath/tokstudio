#!/usr/bin/env bash
# 预览栈共用：先清掉可能凭证漂移的 MinIO 卷，再 docker compose up。
# 若 minio-init 仍失败，再重建卷并重试一次（第二次不再 --build）。
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
  "${COMPOSE[@]}" logs --no-color --tail=200 minio minio-init >&2 || true
  echo "---- end minio logs ----" >&2
}

project_name() {
  local name
  name="$("${COMPOSE[@]}" config --format json 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin).get("name") or "")' 2>/dev/null || true)"
  if [[ -z "$name" ]]; then
    name="$(basename "$(pwd)")"
  fi
  printf '%s' "$name"
}

minio_volume_name() {
  printf '%s_tokenhub_minio' "$(project_name)"
}

# 预览环境对象可丢：先停 MinIO，再删卷，避免卷内旧 root 与 compose 凭证不一致。
recreate_minio_volume() {
  local vol reason="${1:-preview reset}"
  vol="$(minio_volume_name)"
  echo "preview MinIO: ${reason} → recreating volume ${vol}" >&2
  "${COMPOSE[@]}" stop minio minio-init >/dev/null 2>&1 || true
  "${COMPOSE[@]}" rm -f -v minio minio-init >/dev/null 2>&1 || true
  # 卷可能被残留容器占用；尽量再扫一遍同名/标签卷。
  docker volume rm "$vol" >/dev/null 2>&1 || true
  # 兼容旧项目名漂移：列出含 tokenhub_minio 的卷并尝试删除未使用的。
  while read -r extra; do
    [[ -z "$extra" ]] && continue
    [[ "$extra" == "$vol" ]] && continue
    echo "preview MinIO: also removing unused volume ${extra}" >&2
    docker volume rm "$extra" >/dev/null 2>&1 || true
  done < <(docker volume ls -q | grep -E '_tokenhub_minio$' || true)
}

# 预览部署默认先重置卷，避免 mc ready 在错误凭证下把 SSH 超时拖满。
if [[ "${TOKENHUB_PREVIEW_KEEP_MINIO_VOLUME:-}" != "1" ]]; then
  recreate_minio_volume "pre-up credential sync"
fi

if "${COMPOSE[@]}" up --build -d; then
  exit 0
fi

echo "compose up failed; dumping MinIO logs and retrying once" >&2
dump_minio_logs
recreate_minio_volume "post-failure retry"

# 镜像已构建过，重试只拉起，缩短窗口。
if "${COMPOSE[@]}" up -d; then
  echo "compose up succeeded after MinIO volume recreate" >&2
  exit 0
fi

echo "compose up still failing after MinIO volume recreate" >&2
dump_minio_logs
"${COMPOSE[@]}" ps >&2 || true
exit 1
