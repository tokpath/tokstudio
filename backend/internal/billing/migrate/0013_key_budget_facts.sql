ALTER TABLE billing_authorizations ADD COLUMN public_model_id TEXT NOT NULL DEFAULT '';
ALTER TABLE billing_authorizations ADD COLUMN api_key_id TEXT;
ALTER TABLE billing_authorizations ADD COLUMN key_reserved_minor BIGINT NOT NULL DEFAULT 0;
-- Original requests/usage identify the logical Key even after secret rotation.
UPDATE billing_authorizations a SET api_key_id = refs.api_key_id FROM (
 SELECT request_id, max(api_key_id) AS api_key_id FROM (
  SELECT request_id,api_key_id FROM billing_usage_events WHERE api_key_id IS NOT NULL
  UNION ALL SELECT request_id,api_key_id FROM gateway_requests WHERE api_key_id IS NOT NULL
 ) all_refs GROUP BY request_id
) refs WHERE refs.request_id=a.request_id;
UPDATE billing_authorizations SET key_reserved_minor=amount_minor WHERE api_key_id IS NOT NULL AND status IN ('reserved','pending_reconciliation');
UPDATE identity_api_keys k SET budget_used_minor=COALESCE((SELECT sum(c.amount_minor) FROM billing_customer_charges c JOIN billing_usage_events u ON u.id=c.usage_event_id WHERE u.api_key_id=k.id AND c.status='committed'),0),budget_reserved_minor=COALESCE((SELECT sum(a.key_reserved_minor) FROM billing_authorizations a WHERE a.api_key_id=k.id AND a.status IN ('reserved','pending_reconciliation')),0);

UPDATE billing_authorizations a SET public_model_id=r.public_model_id FROM gateway_requests r WHERE r.request_id=a.request_id;
UPDATE billing_authorizations a SET public_model_id=u.public_model_id FROM billing_usage_events u WHERE u.request_id=a.request_id AND a.public_model_id='';
