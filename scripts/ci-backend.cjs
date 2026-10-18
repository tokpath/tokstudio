#!/usr/bin/env node
// Keep the full Go suite and race detector; only distribute execution.
const { execFileSync, spawnSync } = require('node:child_process');
const { appendFileSync } = require('node:fs');
const { resolve } = require('node:path');

const shards = ['portal-1', 'portal-2', 'remediation', 'identity', 'core'];
function packageShard(pkg) {
  if (pkg.endsWith('/internal/portal')) return 'portal';
  if (pkg.endsWith('/internal/remediation')) return 'remediation';
  if (/\/internal\/(organization|incentives)$/.test(pkg)) return 'identity';
  // New packages are included automatically, including packages without tests.
  return 'core';
}
function portalTests(output, shard) {
  if (!['portal-1', 'portal-2'].includes(shard)) throw new Error('Unknown portal shard');
  const names = output.split(/\r?\n/).filter(line => /^(Test|Example|Fuzz)\S+$/.test(line)).sort();
  if (!names.length || new Set(names).size !== names.length) throw new Error('Invalid Go test inventory');
  return names.filter((_, i) => i % 2 === Number(shard.slice(-1)) - 1);
}
function run(shard) {
  if (!shards.includes(shard)) throw new Error(`Expected shard: ${shards.join(', ')}`);
  const cwd = resolve(__dirname, '../backend');
  const capture = args => execFileSync('go', args, { cwd, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
  const packages = capture(['list', './...']).trim().split(/\s+/);
  const selected = packages.filter(pkg => packageShard(pkg) === (shard.startsWith('portal-') ? 'portal' : shard));
  if (!selected.length) throw new Error(`No packages for ${shard}`);
  const args = ['test', '-race', '-p', '1', '-count=1', '-timeout', '15m'];
  let inventory = selected.join('\n');
  if (shard.startsWith('portal-')) {
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
module.exports = { shards, packageShard, portalTests };
if (require.main === module) run(process.argv[2]);
