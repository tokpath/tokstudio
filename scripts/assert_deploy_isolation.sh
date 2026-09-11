#!/usr/bin/env bash
# Sentinel：部署主机、concurrency、检出目录与 test 栈约定。
# 不 SSH、不假装上游已通；只检查仓库约定和本地 dry-run。
# grok 预览栈已移除；本脚本不得再要求 deploy-grok / teardown-grok 产物。
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

[[ -f .github/workflows/deploy.yml ]] || fail "missing deploy.yml"
grep -qF 'secrets.ALIYUN_HOST' .github/workflows/deploy.yml || fail "deploy.yml missing secrets.ALIYUN_HOST"
grep -qF 'secrets.TOKEN_DEPLOY_SSH_KEY' .github/workflows/deploy.yml || fail "deploy.yml missing secrets.TOKEN_DEPLOY_SSH_KEY"
grep -qF 'host: ${{ secrets.ALIYUN_HOST }}' .github/workflows/deploy.yml || fail "deploy.yml host is not secrets.ALIYUN_HOST"
pass "deploy-token pins host and key to secrets"

grep -qF 'group: deploy-token' .github/workflows/deploy.yml || fail "deploy.yml concurrency"
if grep -qF 'group: deploy-grok' .github/workflows/deploy.yml; then
  fail "deploy-token must not use deploy-grok concurrency"
fi
pass "deploy-token concurrency is isolated"

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
if grep -qF 'grok.tokpath.com' .github/workflows/deploy.yml; then
  fail "token workflow must not mention grok.tokpath.com"
fi
pass "token workflow stays on release → test"

# grok 部署 / 拆除产物必须已删除。
for gone in \
  .github/workflows/deploy-grok.yml \
  .github/workflows/teardown-grok.yml \
  docker-compose.grok.yml \
  deploy/caddy-grok.tokpath.com.caddy \
  scripts/deploy_grok.sh \
  scripts/teardown_grok.sh
do
  if [[ -e "$gone" ]]; then
    fail "expected removed: $gone"
  fi
done
pass "grok deploy/teardown artifacts are gone"

# 业务路径不得再出现 grok.tokpath.com（本门禁脚本除外，因断言文案会提到该域名）。
hits="$(grep -R -n -F 'grok.tokpath.com' \
  --exclude-dir=.git \
  --exclude-dir=node_modules \
  --exclude=assert_deploy_isolation.sh \
  README.md DESIGN.md docs backend web deploy scripts .github .env.example Makefile docker-compose*.yml 2>/dev/null || true)"
if [[ -n "$hits" ]]; then
  echo "$hits" >&2
  fail "grok.tokpath.com still referenced outside assert_deploy_isolation.sh"
fi
pass "no grok.tokpath.com outside assert gate"

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
[[ -x scripts/compose_up_with_minio_recovery.sh ]] || fail "compose_up_with_minio_recovery.sh must be executable"
pass "minio credentials + init + deploy recovery wiring"

# dry-run：追加 test 站点，第二次运行幂等；不得引入 grok。
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cat > "$TMP/Caddyfile" <<'EOF'
www.tokpath.com {
	respond "www"
}
EOF
cp .env.example "$TMP/.env"
printf '\nTOKENHUB_PUBLIC_BASE_URL=http://localhost:8080\nTOKENHUB_WEB_ORIGIN=http://localhost:3000\n' >> "$TMP/.env"

CADDY_SNIPPET="$ROOT/deploy/caddy-test.tokpath.com.caddy"
[[ -f "$CADDY_SNIPPET" ]] || fail "missing caddy-test.tokpath.com.caddy"
if ! grep -qF "test.tokpath.com" "$TMP/Caddyfile"; then
  printf "\n" >> "$TMP/Caddyfile"
  cat "$CADDY_SNIPPET" >> "$TMP/Caddyfile"
fi
grep -qF 'test.tokpath.com' "$TMP/Caddyfile" || fail "test.tokpath.com was not appended"
grep -qF 'tokstudio-edge:80' "$TMP/Caddyfile" || fail "test reverse_proxy missing"
if grep -qF 'grok.tokpath.com' "$TMP/Caddyfile"; then
  fail "test caddy dry-run must not mention grok.tokpath.com"
fi

before_hash="$(cksum "$TMP/Caddyfile")"
if ! grep -qF "test.tokpath.com" "$TMP/Caddyfile"; then
  printf "\n" >> "$TMP/Caddyfile"
  cat "$CADDY_SNIPPET" >> "$TMP/Caddyfile"
fi
after_hash="$(cksum "$TMP/Caddyfile")"
[[ "$before_hash" == "$after_hash" ]] || fail "second test append was not idempotent"
test_count="$(grep -cF 'test.tokpath.com {' "$TMP/Caddyfile" || true)"
[[ "$test_count" -eq 1 ]] || fail "expected exactly one test site block, got ${test_count}"

pass "test Caddy append-only + idempotent rerun"
echo "assert_deploy_isolation: all gates passed"
