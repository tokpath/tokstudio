#!/usr/bin/env bash
# 无 Docker 时验证 compose_up_with_minio_recovery.sh：首次 up 失败 → 删卷 → 重试成功。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

PATH_DIR="$TMP/bin"
mkdir -p "$PATH_DIR"

cat > "$PATH_DIR/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
STATE_DIR="${COMPOSE_MOCK_STATE:?}"
mkdir -p "$STATE_DIR"
log="$STATE_DIR/docker.log"
echo "docker $*" >>"$log"

if [[ "${1:-}" == "compose" ]]; then
  shift
  # strip args until a subcommand
  cmd=""
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      -f|-p|--project-name)
        shift 2 || true
        ;;
      --*)
        shift
        ;;
      *)
        cmd="$1"
        shift
        break
        ;;
    esac
  done
  case "$cmd" in
    up)
      attempts_file="$STATE_DIR/up_attempts"
      n=0
      if [[ -f "$attempts_file" ]]; then
        n="$(cat "$attempts_file")"
      fi
      n=$((n + 1))
      echo "$n" >"$attempts_file"
      if [[ "$n" -eq 1 ]]; then
        echo "service \"minio-init\" didn't complete successfully: exit 1" >&2
        exit 1
      fi
      echo "up ok attempt=$n"
      exit 0
      ;;
    logs)
      echo "fake minio-init: Access Denied"
      exit 0
      ;;
    stop|rm)
      echo "$cmd ok" >>"$STATE_DIR/ops.log"
      exit 0
      ;;
    config)
      if [[ "$*" == *"--format json"* ]] || [[ "$*" == *"json"* ]]; then
        echo '{"name":"tokstudio"}'
        exit 0
      fi
      exit 0
      ;;
    ps)
      echo "NAME STATUS"
      exit 0
      ;;
    *)
      echo "unexpected compose cmd: $cmd $*" >&2
      exit 99
      ;;
  esac
fi

if [[ "${1:-}" == "volume" && "${2:-}" == "rm" ]]; then
  echo "volume rm $3" >>"$STATE_DIR/ops.log"
  exit 0
fi

echo "unexpected docker invocation: $*" >&2
exit 99
EOF
chmod +x "$PATH_DIR/docker"

export COMPOSE_MOCK_STATE="$TMP/state"
export PATH="$PATH_DIR:$PATH"

bash "$ROOT/scripts/compose_up_with_minio_recovery.sh" -f docker-compose.yml -f docker-compose.token.yml

attempts="$(cat "$COMPOSE_MOCK_STATE/up_attempts")"
[[ "$attempts" == "2" ]] || {
  echo "expected 2 up attempts, got $attempts" >&2
  exit 1
}
grep -q 'volume rm tokstudio_tokenhub_minio' "$COMPOSE_MOCK_STATE/ops.log" \
  || {
    echo "expected volume recreate" >&2
    cat "$COMPOSE_MOCK_STATE/ops.log" >&2
    exit 1
  }

echo "ok: minio recovery retries once after volume rm"
