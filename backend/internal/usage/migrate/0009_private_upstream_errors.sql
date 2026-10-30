-- Supplier bodies are untrusted and can echo credentials in any format.
-- Preserve numeric statuses and system error codes, not filtered free text.
ALTER TABLE usage_attempts DROP COLUMN provider_error;

-- Existing request reasons previously copied that same supplier body.
UPDATE usage_requests SET reason='upstream_error'
WHERE reason NOT IN (
 '', 'upstream_error', 'upstream_authentication_failed', 'rate_limited',
 'upstream_timeout', 'request_cancelled', 'request_not_dispatched',
 'input_quantity_unavailable', 'route_contract_violation',
 'unsupported_capability', 'invalid_model_request', 'usage_unavailable',
 'usage_bound_exceeded', 'model_execution_failed',
 'usage_unverified_before_deadline', 'upstream_outcome_pending',
 'route_capacity_or_health_unavailable'
);
