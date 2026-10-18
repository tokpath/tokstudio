const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { shards, packageShard, portalTests } = require('./ci-backend.cjs');

test('every existing or future package has exactly one partition', () => {
  for (const name of ['portal', 'remediation', 'organization', 'incentives', 'usage', 'future']) {
    const assigned = shards.filter(shard => packageShard(`example/internal/${name}`) === (shard.startsWith('portal-') ? 'portal' : shard));
    assert.equal(assigned.length, name === 'portal' ? 2 : 1);
  }
  assert.equal(packageShard('example/cmd/new-command'), 'core');
  assert.equal(packageShard('example/internal/portal/subpackage'), 'core');
  const workflow = readFileSync(`${__dirname}/../.github/workflows/ci.yml`, 'utf8');
  assert.ok(workflow.includes(`shard: [${shards.join(', ')}]`));
});
test('portal shards cover all tests, examples and fuzz seeds exactly once, including additions', () => {
  const names = ['TestA', 'TestAB', 'TestNew', 'ExampleUsage', 'FuzzInput'];
  const listing = [...names, 'BenchmarkSpeed', 'ok  example/internal/portal 0.01s', 'diagnostic text'].join('\n');
  const first = portalTests(listing, 'portal-1');
  const second = portalTests(listing, 'portal-2');
  assert.deepEqual([...first, ...second].sort(), [...names].sort());
  assert.equal(new Set([...first, ...second]).size, names.length);
  assert.deepEqual(portalTests([...names].reverse().join('\n'), 'portal-1'), first);
});
test('inventory failures cannot turn a missing test suite green', () => {
  assert.throws(() => portalTests('compile failed', 'portal-1'));
  assert.throws(() => portalTests('TestA\nTestA', 'portal-1'));
  assert.throws(() => portalTests('TestA', 'portal-3'));
});
