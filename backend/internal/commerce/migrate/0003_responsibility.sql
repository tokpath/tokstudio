ALTER TABLE commerce_orders ADD COLUMN original_operator_id UUID REFERENCES organization_operators(id);
ALTER TABLE commerce_orders DROP CONSTRAINT commerce_orders_operator_id_business_key_key;
ALTER TABLE commerce_orders ADD UNIQUE(operator_id,customer_id,business_key);
ALTER TABLE commerce_orders ADD COLUMN original_ownership_version BIGINT;
UPDATE commerce_orders SET original_operator_id=operator_id,original_ownership_version=ownership_version;
ALTER TABLE commerce_orders ALTER COLUMN original_operator_id SET NOT NULL;
ALTER TABLE commerce_orders ALTER COLUMN original_ownership_version SET NOT NULL;
ALTER TABLE commerce_subscriptions ADD COLUMN original_operator_id UUID REFERENCES organization_operators(id);
UPDATE commerce_subscriptions SET original_operator_id=operator_id;
ALTER TABLE commerce_subscriptions ALTER COLUMN original_operator_id SET NOT NULL;
CREATE TABLE commerce_responsibility_transfers (
 takeover_id UUID PRIMARY KEY,from_operator_id UUID NOT NULL,to_operator_id UUID NOT NULL,
 fingerprint TEXT NOT NULL,actor_id TEXT NOT NULL,evidence TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL
);
CREATE FUNCTION commerce_preserve_order_origin() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='INSERT' THEN NEW.original_operator_id:=COALESCE(NEW.original_operator_id,NEW.operator_id); NEW.original_ownership_version:=COALESCE(NEW.original_ownership_version,NEW.ownership_version);
 ELSIF NEW.original_operator_id<>OLD.original_operator_id OR NEW.original_ownership_version<>OLD.original_ownership_version THEN RAISE EXCEPTION 'original order responsibility is immutable' USING ERRCODE='23514'; END IF; RETURN NEW; END $$;
CREATE TRIGGER commerce_order_origin BEFORE INSERT OR UPDATE ON commerce_orders FOR EACH ROW EXECUTE FUNCTION commerce_preserve_order_origin();
CREATE FUNCTION commerce_preserve_subscription_origin() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='INSERT' THEN NEW.original_operator_id:=COALESCE(NEW.original_operator_id,NEW.operator_id);
 ELSIF NEW.original_operator_id<>OLD.original_operator_id THEN RAISE EXCEPTION 'original subscription responsibility is immutable' USING ERRCODE='23514'; END IF; RETURN NEW; END $$;
CREATE TRIGGER commerce_subscription_origin BEFORE INSERT OR UPDATE ON commerce_subscriptions FOR EACH ROW EXECUTE FUNCTION commerce_preserve_subscription_origin();
