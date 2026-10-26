const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');

const root = path.resolve(__dirname, '../..');
const document = JSON.parse(fs.readFileSync(path.join(__dirname, 'tokenhub.json'), 'utf8'));
const metadata = JSON.parse(fs.readFileSync(path.join(__dirname, 'operator-operations.json'), 'utf8'));
const resolve = value => value.$ref ? document.components.schemas[value.$ref.split('/').at(-1)] : value;
const operation = (resource, method = 'post') => document.paths[`/v1/operators/{operatorId}/${resource}`][method];
const body = resource => resolve(operation(resource).requestBody.content['application/json'].schema);

test('workflow target templates become query parameters without inventing path parameters', () => {
  for (const resource of ['branding', 'channel-themes', 'branding/logo', 'coverage']) {
    const action = operation(resource);
    const target = action.parameters.find(parameter => parameter.name === 'target_operator_id');
    assert.equal(target.in, 'query');
    assert.equal(target.schema.format, 'uuid');
    assert.deepEqual(action.parameters.filter(parameter => parameter.in === 'path').map(parameter => parameter.name), ['operatorId']);
  }
  for (const [url, methods] of Object.entries(document.paths)) {
    assert.doesNotMatch(url, /[?#]/);
    const expected = [...url.matchAll(/\{([^}]+)\}/g)].map(match => match[1]).sort();
    for (const action of Object.values(methods)) assert.deepEqual(action.parameters.filter(parameter => parameter.in === 'path').map(parameter => parameter.name).sort(), expected);
  }
});

test('theme inheritance accepts null, preserves version and keeps logo uploads multipart', () => {
  for (const resource of ['branding', 'channel-themes']) {
    const theme = body(resource);
    assert.ok(theme.required.includes('expected_version'));
    const values = theme.properties.values.properties;
    for (const key of ['primary_color', 'background_color', 'text_color', 'logo_url', 'support_url']) assert.deepEqual(values[key].type, ['string', 'null']);
    assert.equal(values.primary_color.pattern, '^#[0-9a-fA-F]{6}$');
    assert.equal(theme.properties.target_operator_id, undefined);
  }
  const upload = operation('branding/logo').requestBody.content;
  assert.equal(upload['application/json'], undefined);
  assert.equal(resolve(upload['multipart/form-data'].schema).properties.file.format, 'binary');
  assert.ok(document.paths['/v1/branding/assets/{id}/content'].get.responses['200'].content['image/png']);
});

test('domain selection uses a body target and explicit DNS/TLS row actions', () => {
  const domain = body('domains');
  assert.deepEqual(domain.required, ['target_operator_id', 'hostname']);
  assert.equal(domain.properties.target_operator_id.format, 'uuid');
  assert.equal(operation('domains').parameters.some(parameter => parameter.name === 'target_operator_id'), false);
  for (const action of ['verify', 'enable', 'disable']) assert.ok(body(`domains/{id}/${action}`).required.includes('version'));
  assert.equal(metadata.some(item => ['branding', 'channel-themes'].includes(item.resource) && item.name === 'domain'), false);
});

test('permissions retain mutation authority for staff and organization creation', () => {
  assert.equal(operation('staff')['x-required-permission'], 'staff.manage');
  assert.equal(operation('staff/{id}/membership')['x-required-permission'], 'staff.manage');
  assert.equal(operation('operators')['x-required-permission'], 'organization.create');
  assert.equal(operation('coverage')['x-required-permission'], 'cooperation.manage');
  assert.equal(operation('branding')['x-required-permission'], 'organization.read');
  assert.ok(metadata.every(item => item.permission));
});

test('model selectors document actual CAS revisions and exact direct-buyer scope', () => {
  const models = operation('references/models', 'get');
  assert.equal(models.parameters.find(parameter => parameter.name === 'buyer_id').in, 'query');
  assert.equal(models.parameters.find(parameter => parameter.name === 'buyer_id').schema.format, 'uuid');
  const defaults = document.components.schemas.ModelReferenceDefaults;
  assert.ok(defaults.required.includes('listing_version'));
  assert.equal(defaults.properties.authorization_version.type, 'integer');
  assert.equal(defaults.required.includes('authorization_version'), false);
  assert.equal(defaults.properties.routing_version.type, 'integer');
  assert.ok(models.parameters.find(parameter => parameter.name === 'purpose'));
  assert.ok(operation('references/branding-brands', 'get').description.includes('channels are excluded'));
});

test('technical capability names match actual runtime operations and output DTOs retain operator context', () => {
  const operations = document.components.schemas.SupplierCapability.properties.operation.enum;
  assert.ok(operations.includes('image_generation'));
  assert.ok(operations.includes('video_generation'));
  assert.equal(operations.includes('image'), false);
  const supplier= document.components.schemas.SupplierModelWrite.properties.supplier_model;
  assert.ok(supplier.properties.base_url);
  assert.equal(supplier.properties.endpoints, undefined);
  assert.equal(supplier.properties.prices, undefined);
  assert.equal(document.components.schemas.SupplierCapability.properties.usage_rules, undefined);
  assert.ok(document.components.schemas.BillingProfile.properties.input);
  assert.ok(document.components.schemas.BillingProfile.properties.output);
  assert.equal(document.components.schemas.RouteLimits, undefined);
  const binding = document.components.schemas.RouteBindingInput.properties;
  assert.equal(binding.routing, undefined);
  assert.equal(binding.priority, undefined);
  assert.ok(binding.supplier_model_id);
  assert.ok(document.components.schemas.OperatorContext.properties.subject_operations);
  assert.ok(document.paths['/v1/operators/{operatorId}/customer-context/{id}']);
  assert.ok(document.paths['/v1/operators/{operatorId}/order-context/{id}/allocate']);
});

test('ledger inputs retain 18-place precision while price inputs and output reading retain 24 places', () => {
  const ledger = new RegExp(document.components.schemas.LedgerDecimal.pattern);
  const price = new RegExp(document.components.schemas.Decimal.pattern);
  assert.ok(ledger.test('0.123456789012345678'));
  assert.equal(ledger.test('0.1234567890123456789'), false);
  assert.ok(price.test('0.123456789012345678901234'));
  assert.equal(document.components.schemas.PriceRule.properties.multiplier.$ref, '#/components/schemas/Decimal');
  assert.equal(document.components.schemas.TopUpInput.properties.amount_usd.$ref, '#/components/schemas/LedgerDecimal');
  const signed = new RegExp(document.components.schemas.SignedLedgerDecimal.pattern);
  assert.ok(signed.test('-0.123456789012345678'));
});

test('risk rules use current rule CAS and operations permission while observations remain a separate read view', () => {
  for (const resource of ['risk', 'alerts']) {
    assert.equal(operation(resource)['x-required-permission'], 'operations.manage');
    const revise = document.components.schemas[`Operator_${resource}_revise_rule`];
    assert.ok(revise.required.includes('id'));
    assert.equal(revise.properties.expected_version.type, 'integer');
    assert.equal(revise.properties.warning.$ref, '#/components/schemas/SignedLedgerDecimal');
    assert.equal(revise.properties.recipients.type, 'array');
  }
});

test('procurement revisions explicitly retain authorization identity and require fresh consent', () => {
  const create = document.components.schemas.Operator_procurement_authorization;
  const revise = document.components.schemas.Operator_procurement_authorization_revision;
  assert.ok(create.required.includes('consent_confirmed'));
  assert.equal(create.properties.single_limit_usd, undefined);
  assert.equal(create.properties.consent_reference, undefined);
  assert.equal(create.properties.id, undefined);
  assert.ok(revise.required.includes('id'));
  assert.ok(revise.required.includes('consent_confirmed'));
  assert.equal(revise.properties.renewed_consent_reference, undefined);
  assert.equal(revise.properties.consent_reference, undefined);
  assert.equal(revise.properties.expected_version.type, 'integer');
  assert.equal(revise.properties.payment.properties.payment_method.writeOnly, true);
  assert.equal(revise.properties.payment.properties.customer_ref.writeOnly, true);
});

test('refund and withdrawal policies retain actual policy version types and nullable review thresholds', () => {
  const refund = document.components.schemas.Operator_refunds_policy;
  assert.equal(refund.properties.action.const, 'policy');
  assert.equal(refund.properties.second_review_above.anyOf[1].type, 'null');
  assert.equal(refund.required.includes('second_review_above'), false);
  assert.equal(refund.properties.expected_version.type, 'integer');
  const withdrawal = document.components.schemas.Operator_withdrawals_policy.properties.configuration;
  assert.equal(withdrawal.properties.expected_version.type, 'integer');
  assert.equal(withdrawal.properties.requires_dual_approval.type, 'boolean');
  assert.equal(withdrawal.properties.fee_rate.$ref, '#/components/schemas/LedgerDecimal');
});

test('generator check is reproducible and does not write reviewed or generated artifacts', () => {
  const files = ['tokenhub.json', 'operator-operations.json'];
  const before = files.map(file => fs.readFileSync(path.join(__dirname, file), 'utf8'));
  const output = execFileSync(process.execPath, [path.join(root, 'scripts/refactor-openapi.cjs'), '--check'], { cwd: root, encoding: 'utf8' });
  assert.match(output, /Verified .* paths, .* schemas and all references/);
  assert.deepEqual(files.map(file => fs.readFileSync(path.join(__dirname, file), 'utf8')), before);
});

test('automatic login and managed staff onboarding do not require an upfront identity or expose passwords', () => {
  assert.deepEqual(document.components.schemas.LoginInput.required, ['email', 'password']);
  const login = document.paths['/v1/auth/login'].post.responses['200'].content['application/json'].schema;
  assert.equal(login.$ref, '#/components/schemas/LoginResult');
  const staff = body('staff');
  assert.equal(staff.properties.initial_password.writeOnly, true);
  assert.equal(document.components.schemas.CommandResult.properties.initial_password, undefined);
  assert.ok(document.paths['/v1/auth/staff-password'].post);
});

test('contracts are private multipart uploads and renewal consent can identify the reviewed quote', () => {
  assert.ok(operation('contracts').requestBody.content['multipart/form-data']);
  assert.ok(operation('contracts/{id}/content', 'get').security.length);
  const renewal = document.paths['/v1/me/plans/{id}/renewal'].patch;
  assert.equal(renewal.requestBody.content['application/json'].schema.properties.quote_id.format, 'uuid');
  assert.ok(document.components.schemas.Quote.properties.terms_summary);
});

test('procurement uses progressive bonus bands and one whole external payment per order', () => {
  const schemas = document.components.schemas;
  const quote = schemas.Operator_procurement_quote;
  assert.equal(quote.properties.price_usd, undefined);
  assert.equal(quote.properties.paid_atg, undefined);
  assert.equal(quote.properties.gift_tiers.type, 'array');
  assert.equal(quote.properties.gift_tiers.items.properties.gift_percent.$ref, '#/components/schemas/LedgerDecimal');
  assert.ok(schemas.Operator_procurement_purchase.required.includes('amount_usd'));
  assert.deepEqual(Object.keys(schemas.Operator_procurement_fund.properties), ['receipt_id']);
  assert.deepEqual(Object.keys(schemas.Operator_procurement_offline_purchase.properties), ['receipt_id']);
  assert.ok(schemas.Operator_procurement_authorization.required.includes('amount_usd'));
});
