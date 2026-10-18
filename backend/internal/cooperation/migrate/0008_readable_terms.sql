ALTER TABLE cooperation_purchase_quotes ADD COLUMN name TEXT NOT NULL DEFAULT '供货报价';
ALTER TABLE cooperation_purchase_quotes ADD COLUMN terms_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE cooperation_purchase_quotes ADD COLUMN contract_attachment_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cooperation_purchase_quotes ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE cooperation_purchase_orders ADD COLUMN name TEXT NOT NULL DEFAULT '采购订单';
ALTER TABLE cooperation_purchase_orders ADD COLUMN terms_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE cooperation_purchase_orders ADD COLUMN contract_attachment_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cooperation_purchase_orders ADD COLUMN quote_version BIGINT NOT NULL DEFAULT 1 CHECK(quote_version>0);
-- Quotes may be enabled/disabled, but accepted commercial facts are immutable.
CREATE FUNCTION cooperation_preserve_purchase_quote() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'purchase quote facts cannot be deleted' USING ERRCODE='23514'; END IF;
 IF ROW(NEW.id,NEW.supplier_id,NEW.buyer_id,NEW.price_usd,NEW.paid_atg,NEW.gift_atg,NEW.exit_terms,NEW.terms_reference,NEW.valid_until,NEW.created_by,NEW.created_at,NEW.name,NEW.terms_summary,NEW.contract_attachment_id,NEW.version) IS DISTINCT FROM ROW(OLD.id,OLD.supplier_id,OLD.buyer_id,OLD.price_usd,OLD.paid_atg,OLD.gift_atg,OLD.exit_terms,OLD.terms_reference,OLD.valid_until,OLD.created_by,OLD.created_at,OLD.name,OLD.terms_summary,OLD.contract_attachment_id,OLD.version) THEN
  RAISE EXCEPTION 'purchase quote commercial facts are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cooperation_purchase_quote_immutable BEFORE UPDATE OR DELETE ON cooperation_purchase_quotes FOR EACH ROW EXECUTE FUNCTION cooperation_preserve_purchase_quote();
CREATE FUNCTION cooperation_preserve_purchase_order_terms() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.supplier_id,NEW.buyer_id,NEW.quote_id,NEW.due_usd,NEW.paid_atg,NEW.gift_atg,NEW.exit_terms,NEW.terms_reference,NEW.name,NEW.terms_summary,NEW.contract_attachment_id,NEW.quote_version) IS DISTINCT FROM ROW(OLD.supplier_id,OLD.buyer_id,OLD.quote_id,OLD.due_usd,OLD.paid_atg,OLD.gift_atg,OLD.exit_terms,OLD.terms_reference,OLD.name,OLD.terms_summary,OLD.contract_attachment_id,OLD.quote_version) THEN
  RAISE EXCEPTION 'purchase order accepted terms are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cooperation_purchase_order_terms_immutable BEFORE UPDATE ON cooperation_purchase_orders FOR EACH ROW EXECUTE FUNCTION cooperation_preserve_purchase_order_terms();
