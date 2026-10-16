CREATE TABLE operations_preferences (
 operator_id UUID NOT NULL, recipient_id UUID NOT NULL,
 marketing BOOLEAN NOT NULL DEFAULT false, usage_email BOOLEAN NOT NULL DEFAULT true,
 updated_at TIMESTAMPTZ NOT NULL, PRIMARY KEY(operator_id,recipient_id)
);
CREATE TABLE operations_notifications (
 id UUID PRIMARY KEY,operator_id UUID NOT NULL,recipient_id UUID NOT NULL,business_key TEXT NOT NULL,
 category TEXT NOT NULL CHECK(category IN ('mandatory','usage','marketing')),
 severity TEXT NOT NULL CHECK(severity IN ('info','warning','critical')),
 title TEXT NOT NULL,body TEXT NOT NULL,reference TEXT NOT NULL,
 fingerprint TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL,read_at TIMESTAMPTZ,
 UNIQUE(operator_id,recipient_id,business_key)
);
CREATE INDEX operations_notifications_inbox ON operations_notifications(operator_id,recipient_id,created_at DESC);
CREATE TABLE operations_deliveries (
 notification_id UUID PRIMARY KEY REFERENCES operations_notifications(id),
 message_ciphertext TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN ('pending','processing','accepted','suppressed','needs_attention')),
 attempts INTEGER NOT NULL DEFAULT 0,next_attempt_at TIMESTAMPTZ NOT NULL,lease_until TIMESTAMPTZ,
 error_code TEXT NOT NULL DEFAULT '',accepted_at TIMESTAMPTZ
);
CREATE TABLE operations_risk_rules (
 operator_id UUID NOT NULL,id TEXT NOT NULL,metric TEXT NOT NULL,
 warning NUMERIC(38,18) NOT NULL,critical NUMERIC(38,18) NOT NULL,direction TEXT NOT NULL CHECK(direction IN ('below','above')),
 recipients JSONB NOT NULL,escalation_recipients JSONB NOT NULL,reminder_seconds BIGINT NOT NULL,escalate_seconds BIGINT NOT NULL,
 enabled BOOLEAN NOT NULL,version BIGINT NOT NULL,updated_by UUID NOT NULL,updated_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(operator_id,id)
);
CREATE TABLE operations_risk_states (
 operator_id UUID NOT NULL,rule_id TEXT NOT NULL,status TEXT NOT NULL,value NUMERIC(38,18) NOT NULL,
 incident BIGINT NOT NULL DEFAULT 0,opened_at TIMESTAMPTZ,last_notified_at TIMESTAMPTZ,sequence BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(operator_id,rule_id)
);
CREATE TABLE operations_reconciliations (
 id UUID PRIMARY KEY,operator_id UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL,created_by UUID NOT NULL,
 finding_count BIGINT NOT NULL
);
CREATE TABLE operations_discrepancies (
 id UUID PRIMARY KEY,run_id UUID NOT NULL REFERENCES operations_reconciliations(id),operator_id UUID NOT NULL,
 domain TEXT NOT NULL,kind TEXT NOT NULL,reference TEXT NOT NULL,unit TEXT NOT NULL,
 expected NUMERIC(38,18) NOT NULL,actual NUMERIC(38,18) NOT NULL,state TEXT NOT NULL DEFAULT 'open',
 adjustment_reference UUID,resolved_by UUID,resolved_at TIMESTAMPTZ,resolution_reason TEXT NOT NULL DEFAULT ''
);
CREATE TABLE operations_exports (
 id UUID PRIMARY KEY,operator_id UUID NOT NULL,requested_by UUID NOT NULL,owner_id UUID,
 kind TEXT NOT NULL,from_at TIMESTAMPTZ NOT NULL,to_at TIMESTAMPTZ NOT NULL,state TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,expires_at TIMESTAMPTZ NOT NULL,lease_until TIMESTAMPTZ,
 content_ciphertext TEXT NOT NULL DEFAULT '',sha256 TEXT NOT NULL DEFAULT '',row_count INTEGER NOT NULL DEFAULT 0,error_code TEXT NOT NULL DEFAULT ''
);
CREATE TABLE operations_export_access (
 id UUID PRIMARY KEY,export_id UUID NOT NULL REFERENCES operations_exports(id),actor_id UUID NOT NULL,at TIMESTAMPTZ NOT NULL
);
CREATE TABLE operations_invoices (
 id UUID PRIMARY KEY,operator_id UUID NOT NULL,customer_id UUID NOT NULL,business_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),state TEXT NOT NULL CHECK(state IN ('requested','issued','rejected','review_required','voided')),
 details_ciphertext TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL,invoice_reference TEXT NOT NULL DEFAULT '',evidence_reference TEXT NOT NULL DEFAULT '',
 handled_by UUID,handled_at TIMESTAMPTZ,reason TEXT NOT NULL DEFAULT '',UNIQUE(operator_id,customer_id,business_key)
);
CREATE TABLE operations_invoice_sources (
 invoice_id UUID NOT NULL REFERENCES operations_invoices(id),receipt_id UUID NOT NULL,
 amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),PRIMARY KEY(invoice_id,receipt_id)
);
CREATE TABLE operations_retention_items (
 id UUID PRIMARY KEY,operator_id UUID NOT NULL,kind TEXT NOT NULL CHECK(kind IN ('temporary_content','technical_record')),
 reference TEXT NOT NULL,payload_ciphertext TEXT NOT NULL,expires_at TIMESTAMPTZ NOT NULL,legal_hold_until TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL,purged_at TIMESTAMPTZ,UNIQUE(operator_id,kind,reference)
);

CREATE TABLE operations_retention_access (
 id UUID PRIMARY KEY,item_id UUID NOT NULL REFERENCES operations_retention_items(id),actor_id UUID NOT NULL,
 action TEXT NOT NULL,reason TEXT NOT NULL,at TIMESTAMPTZ NOT NULL
);
CREATE TABLE operations_invoice_policies(
 operator_id UUID PRIMARY KEY,enabled BOOLEAN NOT NULL,stage TEXT NOT NULL CHECK(stage IN('receipt','top_up','purchase','consumption')),
 maximum_ratio NUMERIC(38,18) NOT NULL CHECK(maximum_ratio>=0 AND maximum_ratio<=1),rules_reference TEXT NOT NULL,
 version BIGINT NOT NULL,updated_by UUID NOT NULL,updated_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE operations_invoices ADD COLUMN policy_stage TEXT NOT NULL;
ALTER TABLE operations_invoices ADD COLUMN policy_version BIGINT NOT NULL;
ALTER TABLE operations_invoices ADD COLUMN policy_ratio NUMERIC(38,18) NOT NULL;
ALTER TABLE operations_invoices ALTER COLUMN customer_id DROP NOT NULL;
ALTER TABLE operations_invoices ADD COLUMN party_kind TEXT NOT NULL DEFAULT 'customer' CHECK(party_kind IN('customer','procurement'));
ALTER TABLE operations_invoices ADD COLUMN buyer_operator_id UUID;
ALTER TABLE operations_invoices ADD COLUMN replaces_invoice_id UUID REFERENCES operations_invoices(id);
ALTER TABLE operations_invoices ADD COLUMN last_reviewed_at TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01';
ALTER TABLE operations_invoices ADD CHECK((party_kind='customer' AND customer_id IS NOT NULL AND buyer_operator_id IS NULL)OR(party_kind='procurement' AND customer_id IS NULL AND buyer_operator_id IS NOT NULL));
CREATE UNIQUE INDEX operations_procurement_invoice_command ON operations_invoices(operator_id,buyer_operator_id,business_key) WHERE party_kind='procurement';
CREATE TABLE operations_invoice_files(
 id UUID PRIMARY KEY,invoice_id UUID NOT NULL REFERENCES operations_invoices(id),operator_id UUID NOT NULL,business_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 object_key TEXT NOT NULL UNIQUE,filename TEXT NOT NULL,content_type TEXT NOT NULL,sha256 TEXT NOT NULL,size_bytes BIGINT NOT NULL,state TEXT NOT NULL CHECK(state IN('uploading','ready')),
 created_by UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL,UNIQUE(invoice_id,business_key)
);
CREATE TABLE operations_invoice_file_access(id UUID PRIMARY KEY,file_id UUID NOT NULL REFERENCES operations_invoice_files(id),invoice_id UUID NOT NULL,actor_id UUID NOT NULL,at TIMESTAMPTZ NOT NULL);
CREATE TABLE operations_invoice_transitions(id UUID PRIMARY KEY,invoice_id UUID NOT NULL REFERENCES operations_invoices(id),from_state TEXT NOT NULL,to_state TEXT NOT NULL,
 invoice_reference TEXT NOT NULL,evidence_reference TEXT NOT NULL,reason TEXT NOT NULL,actor_id UUID NOT NULL,at TIMESTAMPTZ NOT NULL);
CREATE FUNCTION operations_immutable_fact() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'immutable operational fact';END$$;
CREATE TRIGGER operations_invoice_transitions_immutable BEFORE UPDATE OR DELETE ON operations_invoice_transitions FOR EACH ROW EXECUTE FUNCTION operations_immutable_fact();
CREATE TRIGGER operations_invoice_file_access_immutable BEFORE UPDATE OR DELETE ON operations_invoice_file_access FOR EACH ROW EXECUTE FUNCTION operations_immutable_fact();
ALTER TABLE operations_invoice_policies ADD COLUMN party_kind TEXT NOT NULL DEFAULT 'customer' CHECK(party_kind IN('customer','procurement'));
ALTER TABLE operations_invoice_policies DROP CONSTRAINT operations_invoice_policies_pkey;
ALTER TABLE operations_invoice_policies ADD PRIMARY KEY(operator_id,party_kind);
