#!/usr/bin/env bash
# Read-only release guard: only successful release CI can auto-deploy to test.
set -euo pipefail
cd "$(dirname "$0")/.."
python3 - <<'PY'
from pathlib import Path
import re

workflow = Path('.github/workflows/deploy.yml').read_text()
ci = Path('.github/workflows/ci.yml').read_text()
target = Path('scripts/deployment-target.cjs').read_text()
assert re.search(r'^  workflow_dispatch:', workflow, re.M), 'manual dispatch missing'
assert re.search(r'^  workflow_run:', workflow, re.M), 'CI completion trigger missing'
assert not re.search(r'^  (push|pull_request|schedule):', workflow, re.M), 'deployment must wait for successful CI'
required = [
    'workflows: [ci]',
    'types: [completed]',
    'branches: [release/v0.1.0]',
    "github.ref == 'refs/heads/release/v0.1.0'",
    "github.event.workflow_run.conclusion == 'success'",
    "github.event.workflow_run.event == 'push'",
    'github.event.workflow_run.head_repository.full_name == github.repository',
    "require('./scripts/deployment-target.cjs')",
    "if: needs.validate.outputs.deploy == 'true'",
    '[[ "$DEPLOY_SHA" =~ ^[0-9a-f]{40}$ ]]',
    'git merge-base --is-ancestor "$DEPLOY_SHA" origin/release/v0.1.0',
    '[ "$DEPLOY_SHA" != "$(git rev-parse "origin/$BRANCH")" ]',
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
for expected in ["workflow_id: 'ci.yml'", 'head_sha: sha', "runs[0].conclusion !== 'success'", 'current.commit.sha !== sha']:
    assert expected in target, f'CI verification missing: {expected}'
assert 'needs: [changes, backend-tests]' in ci and 'if: always()' in ci, 'backend aggregate gate missing'
assert 'test "$CHANGES_RESULT" = success' in ci, 'change detection must succeed before accepting skipped suites'
assert 'test "$RESULT" = skipped' in ci and 'BACKEND_CHANGED:' in ci, 'unchanged backend must explicitly verify the skipped result'
assert 'test "$RESULT" = success' in ci, 'failed shards must fail the backend check'
assert 'git pull' not in workflow, 'deployment must not advance past the selected SHA'
print('release: successful CI auto-deployment, exact SHA, stale-run guard and test isolation gates present')
PY
