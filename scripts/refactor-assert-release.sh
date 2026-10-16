#!/usr/bin/env bash
# Read-only release guard: CI cannot deploy intermediate refactor pushes.
set -euo pipefail
cd "$(dirname "$0")/.."
python3 - <<'PY'
from pathlib import Path
import re

workflow = Path('.github/workflows/deploy.yml').read_text()
assert re.search(r'^  workflow_dispatch:', workflow, re.M), 'manual dispatch missing'
assert not re.search(r'^  (push|pull_request|workflow_run|schedule):', workflow, re.M), 'automatic deployment trigger present'
required = [
    "github.ref == 'refs/heads/release/v0.1.0'",
    '[[ "$DEPLOY_CONFIRMATION" == "DEPLOY_TEST" ]]',
    '[[ "$DEPLOY_SHA" =~ ^[0-9a-f]{40}$ ]]',
    'git merge-base --is-ancestor "$DEPLOY_SHA" origin/release/v0.1.0',
    "head_sha: process.env.DEPLOY_SHA",
    "runs[0].conclusion !== 'success'",
    'needs: validate',
    'name: tokenhub-test',
    'group: deploy-token',
    'cancel-in-progress: false',
    'git checkout --detach "$DEPLOY_SHA"',
    'test "$(git rev-parse HEAD)" = "$DEPLOY_SHA"',
    'bash scripts/deploy_token.sh',
]
for expected in required:
    assert expected in workflow, f'release gate missing: {expected}'
assert 'git pull' not in workflow, 'deployment must not advance past the selected SHA'
print('refactor release: manual dispatch, exact successful CI SHA, environment and test isolation gates present')
PY
