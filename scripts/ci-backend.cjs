#!/usr/bin/env node
// Keep the full Go suite and race detector; only distribute execution.
// The slow packages are split by test name. Core packages stay whole and are
// divided so one long package cannot share a runner with every other package.
const { execFileSync, spawnSync } = require('node:child_process');
const { appendFileSync } = require('node:fs');
const { resolve } = require('node:path');

const divisions = { portal: 6, remediation: 2, core: 2, organization: 1, incentives: 1, commerce: 1, usage: 1 };
const shards = Object.entries(divisions).flatMap(([group, count]) => count === 1
  ? [group]
  : Array.from({ length: count }, (_, index) => `${group}-${index + 1}`));
function packageShard(pkg) {
  if (pkg.endsWith('/internal/portal')) return 'portal';
  if (pkg.endsWith('/internal/remediation')) return 'remediation';
  if (pkg.endsWith('/internal/organization')) return 'organization';
  if (pkg.endsWith('/internal/incentives')) return 'incentives';
  // These two integration suites dominated one core runner when bundled with
  // each other and all remaining packages. Give each its own infrastructure.
  if (pkg.endsWith('/internal/commerce')) return 'commerce';
  if (pkg.endsWith('/internal/usage')) return 'usage';
  // New packages are included automatically, including packages without tests.
  return 'core';
}
function shardGroup(shard) {
  return shard.replace(/-\d+$/, '');
}
function shardIndex(shard) {
  const count = divisions[shardGroup(shard)];
  if (!count) throw new Error(`Unknown shard: ${shard}`);
  if (count === 1) return { count, index: 0 };
  const index = Number(shard.slice(shard.lastIndexOf('-') + 1));
  if (!Number.isInteger(index) || index < 1 || index > count) throw new Error(`Unknown shard: ${shard}`);
  return { count, index: index - 1 };
}
function packagePart(pkg, count) {
  let hash = 2166136261;
  for (const char of pkg) hash = Math.imul(hash ^ char.charCodeAt(0), 16777619);
  return (hash >>> 0) % count;
}
function packagesForShard(packages, shard) {
  const group = shardGroup(shard);
  const selected = packages.filter(pkg => packageShard(pkg) === group);
  const { count, index } = shardIndex(shard);
  if (count === 1 || divisions[group] && group !== 'core') return selected;
  return selected.filter(pkg => packagePart(pkg, count) === index);
}
function portalTests(output, shard) {
  const { count, index } = shardIndex(shard);
  if (count === 1) throw new Error('Unknown portal shard');
  const names = output.split(/\r?\n/).filter(line => /^(Test|Example|Fuzz)\S+$/.test(line)).sort();
  if (!names.length || new Set(names).size !== names.length) throw new Error('Invalid Go test inventory');
  return names.filter((_, item) => item % count === index);
}
function run(shard) {
  if (!shards.includes(shard)) throw new Error(`Expected shard: ${shards.join(', ')}`);
  const cwd = resolve(__dirname, '../backend');
  const capture = args => execFileSync('go', args, { cwd, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
  const packages = capture(['list', './...']).trim().split(/\s+/);
  const selected = packagesForShard(packages, shard);
  if (!selected.length) throw new Error(`No packages for ${shard}`);
  // Compile dependencies with Go's normal CPU parallelism first. Keep test
  // execution at -p 1 because packages can change shared Redis/S3 fault state.
  // Unlike an empty test run, `list -export` never executes tests or TestMain.
  capture(['list', '-race', '-deps', '-test', '-export', ...selected]);
  const args = ['test', '-race', '-p', '1', '-count=1', '-timeout', '15m'];
  let inventory = selected.join('\n');
  if (shardGroup(shard) === 'portal' || shardGroup(shard) === 'remediation') {
    // Ask Go for runnable tests, examples and fuzz seeds; do not parse source
    // files or rely on a hand-maintained list that can omit future tests.
    const tests = portalTests(capture(['test', '-race', '-list', '.', ...selected]), shard);
    if (!tests.length) throw new Error(`Empty test shard: ${shard}`);
    args.push('-run', `^(${tests.map(name => name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})$`);
    inventory += '\n' + tests.join('\n');
  }
  console.log(`Backend shard ${shard}:\n${inventory}`);
  if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, `### ${shard}\n\n\`\`\`\n${inventory}\n\`\`\`\n`);
  const result = spawnSync('go', [...args, ...selected], { cwd, stdio: 'inherit' });
  if (result.error) throw result.error;
  process.exitCode = result.status ?? 1;
}
module.exports = { shards, packageShard, portalTests, packagesForShard, packagePart };
if (require.main === module) run(process.argv[2]);
