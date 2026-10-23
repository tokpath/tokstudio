#!/usr/bin/env node
// Only skip a suite when every changed path is known to be unrelated to it.
// Missing history, new branches and unrecognised shared files run both suites.
const { execFileSync } = require('node:child_process');
const { appendFileSync } = require('node:fs');

function classify(paths) {
  if (!paths.length) return { backend: true, frontend: true };
  let backend = false, frontend = false;
  for (const path of paths) {
    if (path.startsWith('backend/')) {
      backend = true;
      if (path.startsWith('backend/openapi/')) frontend = true;
    } else if (path.startsWith('web/')) {
      frontend = true;
      // These sources also generate messages embedded in the Go API.
      if (path.startsWith('web/locales/') || path === 'web/scripts/sync-i18n.mjs') backend = true;
    } else if (path.startsWith('docs/') || /^(README|DESIGN|AGENTS|CHANGELOG)\.md$/.test(path)) {
      continue;
    } else {
      return { backend: true, frontend: true };
    }
  }
  return { backend, frontend };
}

function changes(base, head, capture = execFileSync) {
  if (![base, head].every(sha => /^[0-9a-f]{40}$/.test(sha || '') && !/^0+$/.test(sha))) {
    return { backend: true, frontend: true };
  }
  try {
    // A move across directories affects both its old and new scopes.
    const paths = capture('git', ['diff', '--name-only', '--no-renames', '-z', base, head], { encoding: 'utf8' }).split('\0').filter(Boolean);
    return classify(paths);
  } catch {
    return { backend: true, frontend: true };
  }
}

module.exports = { classify, changes };
if (require.main === module) {
  const result = changes(process.env.CI_BASE_SHA, process.env.CI_HEAD_SHA);
  for (const [key, value] of Object.entries(result)) {
    console.log(`${key}=${value}`);
    if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${key}=${value}\n`);
  }
}
