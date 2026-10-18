-- Private cooperation documents are distinct from financial verification evidence.
-- Content and ownership never change after upload; pending uploads can only be
-- completed with the same fingerprint and object key.
CREATE TABLE organization_cooperation_contracts (
 id UUID PRIMARY KEY,
 operator_id UUID NOT NULL REFERENCES organization_operators(id),
 business_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 object_key TEXT NOT NULL UNIQUE,
 filename TEXT NOT NULL,
 content_type TEXT NOT NULL CHECK(content_type IN('application/pdf','image/png','image/jpeg')),
 size_bytes INT NOT NULL CHECK(size_bytes>0 AND size_bytes<=10485760),
 status TEXT NOT NULL CHECK(status IN('pending','ready')),
 created_by UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(operator_id,business_key)
);
CREATE FUNCTION organization_preserve_cooperation_contract() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'cooperation contracts cannot be deleted' USING ERRCODE='23514'; END IF;
 IF ROW(NEW.id,NEW.operator_id,NEW.business_key,NEW.fingerprint,NEW.object_key,NEW.filename,NEW.content_type,NEW.size_bytes,NEW.created_by,NEW.created_at) IS DISTINCT FROM ROW(OLD.id,OLD.operator_id,OLD.business_key,OLD.fingerprint,OLD.object_key,OLD.filename,OLD.content_type,OLD.size_bytes,OLD.created_by,OLD.created_at) OR (OLD.status='ready' AND NEW.status<>'ready') THEN
  RAISE EXCEPTION 'cooperation contract content and ownership are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER organization_cooperation_contract_immutable BEFORE UPDATE OR DELETE ON organization_cooperation_contracts FOR EACH ROW EXECUTE FUNCTION organization_preserve_cooperation_contract();
ALTER TABLE organization_supply_agreements ADD COLUMN supply_terms TEXT NOT NULL DEFAULT '';
ALTER TABLE organization_supply_agreements ADD COLUMN exit_policy TEXT NOT NULL DEFAULT 'non_refundable' CHECK(exit_policy IN('non_refundable','remaining_paid_proportional'));
ALTER TABLE organization_supply_agreements ADD COLUMN contract_attachment_id TEXT NOT NULL DEFAULT '';
ALTER TABLE organization_supply_agreements ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK(version>0);
