CREATE TABLE remediation_takeovers (
 id uuid PRIMARY KEY,channel_id uuid NOT NULL REFERENCES organization_operators(id),brand_operator_id uuid NOT NULL REFERENCES organization_operators(id),business_key text NOT NULL,
 fingerprint text NOT NULL,state text NOT NULL CHECK(state IN('preparing','awaiting_approval','executing','completed','cancelled')),
 method text NOT NULL CHECK(method IN('supply_coverage','agreed_atg_value')),service_usd_per_atg numeric(38,18) NOT NULL CHECK(service_usd_per_atg>0),
 procurement_usd_per_atg numeric(38,18) NOT NULL CHECK(procurement_usd_per_atg>0),extra_usd numeric(38,18) NOT NULL,
 contract_reference text NOT NULL,evidence text NOT NULL,requested_by uuid NOT NULL,approved_by uuid,
 previous_status jsonb NOT NULL,status_version bigint NOT NULL,snapshot jsonb,snapshot_fingerprint text NOT NULL DEFAULT '',
 service_usd numeric(38,18) NOT NULL DEFAULT 0,cash_usd numeric(38,18) NOT NULL DEFAULT 0,procurement_offset_atg numeric(38,18) NOT NULL DEFAULT 0,
 procurement_offset_usd numeric(38,18) NOT NULL DEFAULT 0,remaining_procurement_refund_usd numeric(38,18) NOT NULL DEFAULT 0,
 settlement_due_usd numeric(38,18) NOT NULL DEFAULT 0,settled_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(settled_usd>=0),
 created_at timestamptz NOT NULL,completed_at timestamptz,UNIQUE(channel_id,business_key)
);
CREATE UNIQUE INDEX remediation_channel_one_takeover ON remediation_takeovers(channel_id) WHERE state<>'cancelled';
CREATE TABLE remediation_takeover_settlements(id uuid PRIMARY KEY,takeover_id uuid NOT NULL REFERENCES remediation_takeovers(id),business_key text NOT NULL,fingerprint text NOT NULL,amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0),fee_usd numeric(38,18) NOT NULL CHECK(fee_usd>=0),currency text NOT NULL,original_amount text NOT NULL,reference text NOT NULL,evidence text NOT NULL,occurred_at timestamptz NOT NULL,created_at timestamptz NOT NULL,UNIQUE(takeover_id,business_key),UNIQUE(takeover_id,reference));
CREATE TRIGGER remediation_takeover_settlements_immutable BEFORE UPDATE OR DELETE ON remediation_takeover_settlements FOR EACH ROW EXECUTE FUNCTION remediation_immutable();
CREATE TABLE remediation_event_publications(event_id uuid PRIMARY KEY REFERENCES remediation_events(id),published_at timestamptz NOT NULL);
