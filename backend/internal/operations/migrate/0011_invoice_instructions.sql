-- Eligibility follows completed orders. Retire obsolete optional-service gates.
-- Preserve actual invoices, applications and their files; discard old rule codes.
CREATE TABLE operations_invoice_instructions (
 operator_id UUID PRIMARY KEY,
 instructions TEXT NOT NULL DEFAULT '',
 version BIGINT NOT NULL,
 updated_by UUID NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE operations_invoices ADD COLUMN instructions TEXT NOT NULL DEFAULT '';
ALTER TABLE operations_invoices DROP COLUMN policy_stage, DROP COLUMN policy_version, DROP COLUMN policy_ratio;
DROP TABLE operations_invoice_policy_versions;
DROP TABLE operations_invoice_policies;
