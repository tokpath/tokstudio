const { test } = require('node:test');
const assert = require('node:assert/strict');
const { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, readFileSync, rmSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join, resolve } = require('node:path');
const { spawnSync } = require('node:child_process');

function deployment(t, failure = '') {
  const root = mkdtempSync(join(tmpdir(), 'tokenhub-deploy-build-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const scripts = join(root, 'scripts'), bin = join(root, 'bin'), log = join(root, 'docker.log');
  mkdirSync(scripts); mkdirSync(bin);
  for (const name of ['deploy_token.sh', 'compose_up_with_minio_recovery.sh']) {
    copyFileSync(resolve(__dirname, name), join(scripts, name));
  }
  writeFileSync(join(root, '.env'), '');
  const mock = (name, source) => writeFileSync(join(bin, name), `#!/usr/bin/env bash\nset -eu\n${source}\n`, { mode: 0o755 });
  mock('docker', `printf '%s\\n' "$*" >> "$DEPLOY_BUILD_LOG"
case " $* " in
  *" build "*) if [ "$DEPLOY_BUILD_FAILURE" = build ]; then exit 71; fi ;;
  *" run --rm migrate "*) if [ "$DEPLOY_BUILD_FAILURE" = migrate ]; then exit 72; fi ;;
esac`);
  mock('df', 'printf "Filesystem 1024-blocks Used Available Capacity Mounted\\nmock 20000000 1000000 19000000 5%% /\\n"');
  mock('curl', 'exit 0');
  const result = spawnSync('bash', [join(scripts, 'deploy_token.sh')], {
    encoding: 'utf8',
    env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, NOVA_CADDY: join(root, 'absent-caddy'), DEPLOY_BUILD_LOG: log, DEPLOY_BUILD_FAILURE: failure, TOKENHUB_RESET_PREVIEW_MINIO_VOLUME: '0' },
  });
  return { result, commands: readFileSync(log, 'utf8').trim().split('\n') };
}

test('deployment builds once, migrates, then starts the already-built images', t => {
  const { result, commands } = deployment(t);
  assert.equal(result.status, 0, result.stderr);
  const builds = commands.filter(line => line.includes(' build '));
  assert.equal(builds.length, 1);
  assert.ok(builds[0].endsWith(' build api web backup'));
  const migration = commands.findIndex(line => line.endsWith(' run --rm migrate'));
  const startup = commands.findIndex(line => line.endsWith(' up --no-build -d'));
  assert.ok(migration > commands.indexOf(builds[0]));
  assert.ok(startup > migration);
  assert.equal(commands.some(line => line.includes(' up --build ')), false);
});
for (const failure of ['build', 'migrate']) test(`a failed ${failure} cannot replace running services`, t => {
  const { result, commands } = deployment(t, failure);
  assert.notEqual(result.status, 0);
  assert.equal(commands.some(line => line.includes(' up --no-build ')), false);
  assert.equal(commands.some(line => /volume rm|\bdown\b/.test(line)), false);
});
