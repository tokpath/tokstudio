CREATE TABLE operations_invoice_policy_versions(
 operator_id UUID NOT NULL,party_kind TEXT NOT NULL CHECK(party_kind IN('customer','procurement')),version BIGINT NOT NULL CHECK(version>0),
 snapshot JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL,PRIMARY KEY(operator_id,party_kind,version)
);
INSERT INTO operations_invoice_policy_versions(operator_id,party_kind,version,snapshot,created_at)
 SELECT p.operator_id,p.party_kind,p.version,to_jsonb(p),p.updated_at FROM operations_invoice_policies p;
CREATE TRIGGER operations_invoice_policy_versions_immutable BEFORE UPDATE OR DELETE ON operations_invoice_policy_versions FOR EACH ROW EXECUTE FUNCTION operations_immutable_fact();
