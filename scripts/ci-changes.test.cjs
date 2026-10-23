const { test } = require('node:test');
const assert = require('node:assert/strict');
const { classify, changes } = require('./ci-changes.cjs');
const all = { backend: true, frontend: true };
test('frontend-only changes skip Go tests, but shared translations do not', () => {
  assert.deepEqual(classify(['web/components/page.tsx', 'web/styles/globals.css']), { backend: false, frontend: true });
  assert.deepEqual(classify(['web/locales/forms.json']), all);
  assert.deepEqual(classify(['web/scripts/sync-i18n.mjs']), all);
});
test('backend changes retain Go tests and API contracts also check the web', () => {
  assert.deepEqual(classify(['backend/internal/commerce/orders.go']), { backend: true, frontend: false });
  assert.deepEqual(classify(['backend/openapi/tokenhub.json']), all);
  assert.deepEqual(classify(['backend/internal/portal/commands.go', 'web/components/operation-form.tsx']), all);
  assert.deepEqual(classify(['backend/removed.go', 'docs/moved.go']), { backend: true, frontend: false });
});
test('documentation can skip suites; unknown infrastructure always runs both', () => {
  assert.deepEqual(classify(['docs/product-spec/spec.md', 'README.md']), { backend: false, frontend: false });
  for (const path of ['.github/workflows/ci.yml', 'docker-compose.yml', 'scripts/deploy_token.sh', 'future.lock']) {
    assert.deepEqual(classify([path]), all);
  }
  assert.deepEqual(classify([]), all);
});
test('missing history or invalid revisions cannot silently skip tests', () => {
  const head = 'a'.repeat(40), base = 'b'.repeat(40);
  for (const missing of [undefined, '', '0'.repeat(40), 'bad; echo unsafe']) {
    assert.deepEqual(changes(missing, head, () => { throw new Error('must not execute'); }), all);
  }
  assert.deepEqual(changes(base, head, () => { throw new Error('missing ancestor'); }), all);
  assert.deepEqual(changes(base, head, (_file, args) => {
    assert.deepEqual(args, ['diff', '--name-only', '--no-renames', '-z', base, head]);
    return 'web/name with spaces.tsx\0web/deleted.tsx\0';
  }), { backend: false, frontend: true });
});
