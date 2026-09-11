#!/usr/bin/env bash
# Sentinel：部署主机、concurrency、检出目录与 nova Caddy 追加不得破坏 test 栈。
# 不 SSH、不假装上游已通；只检查仓库约定和本地 dry-run。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

pass() {
  echo "ok: $*"
}

if grep -R -n --include='*.yml' --include='*.yaml' -F '47.237.102.13' .github/workflows; then
  fail "hardcoded 47.237.102.13 still in workflows"
fi
pass "no literal 47.237.102.13 in workflows"

if grep -R -n --include='*.yml' --include='*.yaml' -F '${{ vars.ALIYUN_HOST }}' .github/workflows; then
  fail "workflows must use secrets.ALIYUN_HOST, not vars.ALIYUN_HOST"
fi
pass 'no ${{ vars.ALIYUN_HOST }} usage in workflows'

for f in .github/workflows/deploy.yml .github/workflows/deploy-grok.yml; do
  [[ -f "$f" ]] || fail "missing $f"
  grep -qF 'secrets.ALIYUN_HOST' "$f" || fail "$f missing secrets.ALIYUN_HOST"
  grep -qF 'secrets.TOKEN_DEPLOY_SSH_KEY' "$f" || fail "$f missing secrets.TOKEN_DEPLOY_SSH_KEY"
  grep -qF 'host: ${{ secrets.ALIYUN_HOST }}' "$f" || fail "$f host is not secrets.ALIYUN_HOST"
done
pass "both workflows pin host and key to secrets"

grep -qF 'group: deploy-token' .github/workflows/deploy.yml || fail "deploy.yml concurrency"
grep -qF 'group: deploy-grok' .github/workflows/deploy-grok.yml || fail "deploy-grok.yml concurrency"
if grep -qF 'group: deploy-token' .github/workflows/deploy-grok.yml; then
  fail "deploy-grok shares deploy-token concurrency"
fi
if grep -qF 'group: deploy-grok' .github/workflows/deploy.yml; then
  fail "deploy-token shares deploy-grok concurrency"
fi
pass "concurrency groups are isolated"

grep -qF 'release/v0.1.0' .github/workflows/deploy.yml || fail "token workflow must gate on release/v0.1.0"
grep -qF '/root/workspace/tokstudio' .github/workflows/deploy.yml || fail "token workflow must use test checkout"
if grep -qF 'feature/grokbot' .github/workflows/deploy.yml; then
  fail "token workflow must not mention feature/grokbot"
fi
if grep -qF 'tokstudio-grok' .github/workflows/deploy.yml; then
  fail "token workflow must not mention tokstudio-grok"
fi
if grep -qF 'deploy_grok.sh' .github/workflows/deploy.yml; then
  fail "token workflow must not call deploy_grok.sh"
fi
pass "token workflow stays on release → test"

grep -qF 'feature/grokbot' .github/workflows/deploy-grok.yml || fail "grok workflow must gate on feature/grokbot"
grep -qF '/root/workspace/tokstudio-grok' .github/workflows/deploy-grok.yml || fail "grok workflow must use isolated checkout"
grep -qF 'deploy_grok.sh' .github/workflows/deploy-grok.yml || fail "grok workflow must call deploy_grok.sh"
if grep -qF 'deploy_token.sh' .github/workflows/deploy-grok.yml; then
  fail "grok workflow must not call deploy_token.sh"
fi
if grep -nE '[[:space:]]ROOT=/root/workspace/tokstudio[[:space:]]*$' .github/workflows/deploy-grok.yml; then
  fail "grok workflow must not check out into the test tree"
fi
pass "grok workflow stays on feature/grokbot → isolated dir"

grep -qF 'name: tokstudio-grok' docker-compose.grok.yml || fail "compose project name"
grep -qF 'tokstudio-grok-edge' docker-compose.grok.yml || fail "grok edge service name"
grep -qF 'https://grok.tokpath.com' docker-compose.grok.yml || fail "grok public URL in compose"
grep -qF 'tokstudio-grok-edge:80' deploy/caddy-grok.tokpath.com.caddy || fail "caddy must proxy grok edge"
if grep -vE '^[[:space:]]*#' deploy/caddy-grok.tokpath.com.caddy | grep -qF 'test.tokpath.com'; then
  fail "grok caddy snippet must not mention test.tokpath.com"
fi
if grep -qF 'tokstudio-edge:80' deploy/caddy-grok.tokpath.com.caddy; then
  fail "grok caddy snippet must not point at the test edge"
fi
# Google mock 开关已移除；确认 grok overlay 不再引用。
if grep -vE '^[[:space:]]*#' docker-compose.grok.yml | grep -q 'TOKENHUB_GOOGLE_ALLOW_MOCK\|TOKENHUB_BIFROST_SANDBOX'; then
  fail "grok compose must not reference removed mock/sandbox flags"
fi
pass "grok compose + caddy stay isolated"

# MinIO：凭证走 TOKENHUB_S3_*；init 用 mc ready；部署走卷重建恢复脚本。
grep -qF 'MINIO_ROOT_USER: ${TOKENHUB_S3_ACCESS_KEY:-minioadmin}' docker-compose.yml \
  || fail "minio root user must track TOKENHUB_S3_ACCESS_KEY"
grep -qF 'MINIO_ROOT_PASSWORD: ${TOKENHUB_S3_SECRET_KEY:-minioadmin}' docker-compose.yml \
  || fail "minio root password must track TOKENHUB_S3_SECRET_KEY"
grep -qF 'mc ready local' docker-compose.yml || fail "minio-init must wait with mc ready"
if grep -A2 'minio:$' docker-compose.yml | grep -q 'curl'; then
  fail "minio healthcheck must not rely on curl (server image has none)"
fi
grep -qF 'compose_up_with_minio_recovery.sh' scripts/deploy_token.sh \
  || fail "deploy_token.sh must use MinIO volume recovery helper"
grep -qF 'compose_up_with_minio_recovery.sh' scripts/deploy_grok.sh \
  || fail "deploy_grok.sh must use MinIO volume recovery helper"
[[ -x scripts/compose_up_with_minio_recovery.sh ]] || fail "compose_up_with_minio_recovery.sh must be executable"
pass "minio credentials + init + deploy recovery wiring"

# 目录门禁：误在 test 检出里跑 grok 必须失败。
if TOKENHUB_TEST_CHECKOUT="$ROOT" TOKENHUB_DEPLOY_SKIP_DOCKER=1 TOKENHUB_DEPLOY_SKIP_ENV=1 \
  bash scripts/deploy_grok.sh 2>/tmp/grok-refuse.err; then
  fail "deploy_grok.sh should refuse the test checkout"
fi
grep -q 'separate checkout' /tmp/grok-refuse.err || fail "missing refuse message for test checkout"
pass "deploy_grok.sh refuses the test checkout"

# dry-run：追加 grok 站点，test 块保持原样，第二次运行幂等。
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cat > "$TMP/Caddyfile" <<'EOF'
www.tokpath.com {
	respond "www"
}

test.tokpath.com {
	encode zstd gzip
	reverse_proxy tokstudio-edge:80 {
		header_up Host {host}
		header_up X-Real-IP {remote_host}
	}
}
EOF
cp .env.example "$TMP/.env"
# 给 test .env 一个可被覆盖的对照值，确认 grok 只写 grok URL。
printf '\nTOKENHUB_PUBLIC_BASE_URL=https://test.tokpath.com\nTOKENHUB_WEB_ORIGIN=https://test.tokpath.com\n' >> "$TMP/.env"

TOKENHUB_DEPLOY_SKIP_DOCKER=1 \
  TOKENHUB_ENV_FILE="$TMP/.env" \
  NOVA_CADDY="$TMP/Caddyfile" \
  bash scripts/deploy_grok.sh

grep -qF 'test.tokpath.com' "$TMP/Caddyfile" || fail "test.tokpath.com block was removed"
grep -qF 'tokstudio-edge:80' "$TMP/Caddyfile" || fail "test reverse_proxy was edited"
grep -qF 'grok.tokpath.com' "$TMP/Caddyfile" || fail "grok.tokpath.com was not appended"
grep -qF 'tokstudio-grok-edge:80' "$TMP/Caddyfile" || fail "grok reverse_proxy missing"
test_count="$(grep -cF 'test.tokpath.com' "$TMP/Caddyfile")"
grok_count="$(grep -cF 'grok.tokpath.com {' "$TMP/Caddyfile" || true)"
[[ "$test_count" -ge 1 ]] || fail "test host vanished"
[[ "$grok_count" -eq 1 ]] || fail "expected exactly one grok site block, got ${grok_count}"

grep -qF 'TOKENHUB_PUBLIC_BASE_URL=https://grok.tokpath.com' "$TMP/.env" || fail "PUBLIC_BASE_URL not grok"
grep -qF 'TOKENHUB_WEB_ORIGIN=https://grok.tokpath.com' "$TMP/.env" || fail "WEB_ORIGIN not grok"
if grep -qF 'TOKENHUB_PUBLIC_BASE_URL=https://test.tokpath.com' "$TMP/.env"; then
  fail "grok .env still points PUBLIC_BASE at test"
fi

before_hash="$(cksum "$TMP/Caddyfile")"
TOKENHUB_DEPLOY_SKIP_DOCKER=1 TOKENHUB_DEPLOY_SKIP_ENV=1 \
  NOVA_CADDY="$TMP/Caddyfile" \
  bash scripts/deploy_grok.sh
after_hash="$(cksum "$TMP/Caddyfile")"
[[ "$before_hash" == "$after_hash" ]] || fail "second grok append was not idempotent"
grok_count="$(grep -cF 'grok.tokpath.com {' "$TMP/Caddyfile" || true)"
[[ "$grok_count" -eq 1 ]] || fail "idempotent rerun duplicated grok site block"

pass "nova Caddy append-only + grok .env + idempotent rerun"
echo "assert_deploy_isolation: all gates passed"
