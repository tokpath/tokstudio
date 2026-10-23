const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { shards, packageShard, portalTests, packagesForShard } = require('./ci-backend.cjs');

test('every existing or future package has exactly one partition', () => {
  const counts = { portal: 6, remediation: 2, organization: 1, incentives: 1, commerce: 1, usage: 1, future: 2 };
  for (const [name, count] of Object.entries(counts)) {
    const assigned = shards.filter(shard => packageShard(`example/internal/${name}`) === shard.replace(/-\d+$/, ''));
    assert.equal(assigned.length, count, name);
  }
  assert.equal(packageShard('example/cmd/new-command'), 'core');
  assert.equal(packageShard('example/internal/portal/subpackage'), 'core');
  const workflow = readFileSync(`${__dirname}/../.github/workflows/ci.yml`, 'utf8');
  assert.ok(workflow.includes(`shard: [${shards.join(', ')}]`));
});
test('portal and remediation shards cover every test exactly once', () => {
  const names = ['TestA', 'TestAB', 'TestNew', 'ExampleUsage', 'FuzzInput'];
  const listing = [...names, 'BenchmarkSpeed', 'ok  example/internal/portal 0.01s', 'diagnostic text'].join('\n');
  for (const group of ['portal', 'remediation']) {
    const parts = shards.filter(shard => shard.startsWith(`${group}-`)).map(shard => portalTests(listing, shard));
    const covered = parts.flat();
    assert.deepEqual(covered.sort(), [...names].sort());
    assert.equal(new Set(covered).size, names.length);
  }
  assert.deepEqual(portalTests([...names].reverse().join('\n'), 'portal-1'), portalTests(listing, 'portal-1'));
});
test('core packages are divided without dropping or repeating one', () => {
  const packages = ['example/internal/catalog', 'example/internal/payments', 'example/cmd/api', 'example/internal/modelruntime', 'example/internal/ledger'];
  const covered = shards.filter(shard => shard.startsWith('core-')).flatMap(shard => packagesForShard(packages, shard));
  assert.deepEqual(covered.sort(), [...packages].sort());
  assert.equal(new Set(covered).size, packages.length);
  assert.deepEqual(packagesForShard(['example/internal/organization'], 'organization'), ['example/internal/organization']);
  for (const group of ['commerce', 'usage']) {
    const packages = [`example/internal/${group}`, 'example/internal/catalog'];
    assert.deepEqual(shards.flatMap(shard => packagesForShard(packages, shard)).sort(), [...packages].sort());
    assert.deepEqual(packagesForShard(packages, group), [packages[0]]);
  }
});
test('inventory failures cannot turn a missing test suite green', () => {
  assert.throws(() => portalTests('compile failed', 'portal-1'));
  assert.throws(() => portalTests('TestA\nTestA', 'portal-1'));
  assert.throws(() => portalTests('TestA', 'portal-7'));
  assert.throws(() => portalTests('TestA', 'organization'));
});
