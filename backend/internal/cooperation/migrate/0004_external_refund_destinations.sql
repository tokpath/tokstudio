CREATE TABLE cooperation_refund_destination_proposals(
 id UUID PRIMARY KEY,leg_id UUID NOT NULL REFERENCES cooperation_procurement_exit_refunds(id),buyer_id UUID NOT NULL,supplier_id UUID NOT NULL,
 destination JSONB NOT NULL,payer_authorization_reference TEXT NOT NULL,proposed_by UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE cooperation_refund_destination_authorizations(
 id UUID PRIMARY KEY,proposal_id UUID NOT NULL UNIQUE REFERENCES cooperation_refund_destination_proposals(id),leg_id UUID NOT NULL UNIQUE REFERENCES cooperation_procurement_exit_refunds(id),
 buyer_id UUID NOT NULL,supplier_id UUID NOT NULL,destination JSONB NOT NULL,verified_by UUID NOT NULL,created_at TIMESTAMPTZ NOT NULL
);
CREATE TRIGGER cooperation_refund_destination_proposals_immutable BEFORE UPDATE OR DELETE ON cooperation_refund_destination_proposals FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
CREATE TRIGGER cooperation_refund_destination_authorizations_immutable BEFORE UPDATE OR DELETE ON cooperation_refund_destination_authorizations FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
