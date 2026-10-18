const { test } = require('node:test');
const assert = require('node:assert/strict');
const validate = require('./deployment-target.cjs');
const sha = 'a'.repeat(40);
const newer = 'b'.repeat(40);
function fixture() {
  const outputs = {};
  const run = { id: 7, run_number: 7, run_attempt: 1, head_sha: sha, event: 'push', head_branch: 'release/v0.1.0',
    head_repository: { full_name: 'tokpath/tokstudio' }, status: 'completed', conclusion: 'success' };
  const context = { repo: { owner: 'tokpath', repo: 'tokstudio' }, eventName: 'workflow_run',
    ref: 'refs/heads/release/v0.1.0', payload: { workflow_run: structuredClone(run) } };
  const state = { current: sha, runs: [run], requested: null };
  const github = { rest: {
    repos: { getBranch: async () => ({ data: { commit: { sha: state.current } } }) },
    actions: { listWorkflowRuns: async args => { state.requested = args; return { data: { workflow_runs: state.runs } }; } },
  } };
  const core = { setOutput: (key, value) => { outputs[key] = value; }, notice: () => {} };
  return { github, context, core, state, outputs };
}
test('successful release CI selects the tested SHA, not the workflow default-branch SHA', async () => {
  const f = fixture(); f.context.sha = newer;
  await validate(f);
  assert.deepEqual(f.outputs, { deploy: 'true', sha });
  assert.equal(f.state.requested.head_sha, sha);
  assert.equal(f.state.requested.workflow_id, 'ci.yml');
});
test('superseded automatic runs skip publication', async () => {
  const f = fixture(); f.state.current = newer;
  await validate(f);
  assert.deepEqual(f.outputs, { deploy: 'false' });
});
for (const [name, changes] of [
  ['failed CI', { conclusion: 'failure' }], ['cancelled CI', { conclusion: 'cancelled' }],
  ['unfinished CI', { status: 'in_progress' }], ['pull request', { event: 'pull_request' }],
  ['another branch', { head_branch: 'main' }], ['fork', { head_repository: { full_name: 'fork/tokstudio' } }],
  ['invalid SHA', { head_sha: '$(touch bad)' }],
]) test(`rejects ${name}`, async () => {
  const f = fixture(); Object.assign(f.context.payload.workflow_run, changes);
  await assert.rejects(validate(f)); assert.equal(f.outputs.deploy, 'false');
});
test('latest attempt must succeed, even if a previous attempt succeeded', async () => {
  const f = fixture();
  f.state.runs.push({ ...f.state.runs[0], run_attempt: 2, status: 'in_progress', conclusion: null });
  await assert.rejects(validate(f)); assert.equal(f.outputs.deploy, 'false');
});
test('manual recovery still requires explicit confirmation and successful CI for that SHA', async () => {
  const f = fixture(); f.context.eventName = 'workflow_dispatch'; f.state.current = newer;
  f.context.payload.inputs = { commit_sha: sha, confirmation: 'DEPLOY_TEST' };
  await validate(f); assert.deepEqual(f.outputs, { deploy: 'true', sha });
  f.context.payload.inputs.confirmation = '';
  await assert.rejects(validate(f)); assert.equal(f.outputs.deploy, 'false');
  f.context.payload.inputs.confirmation = 'DEPLOY_TEST'; f.state.runs = [];
  await assert.rejects(validate(f)); assert.equal(f.outputs.deploy, 'false');
});
test('manual dispatch from other branches and unsupported events are rejected', async () => {
  const f = fixture(); f.context.eventName = 'workflow_dispatch'; f.context.ref = 'refs/heads/main';
  f.context.payload.inputs = { commit_sha: sha, confirmation: 'DEPLOY_TEST' };
  await assert.rejects(validate(f));
  f.context.eventName = 'push'; await assert.rejects(validate(f));
});
