-- Preserve accepted fixed quotes as read-only evidence, never reinterpret them
-- as a different offer or charge an old authorization with a new purchase amount.
DROP TRIGGER cooperation_purchase_quote_immutable ON cooperation_purchase_quotes;
ALTER TABLE cooperation_purchase_quotes ADD COLUMN gift_tiers JSONB;
ALTER TABLE cooperation_purchase_quotes ADD COLUMN legacy_pricing JSONB;
UPDATE cooperation_purchase_quotes SET legacy_pricing=jsonb_build_object('price_usd',price_usd::text,'paid_atg',paid_atg::text,'gift_atg',gift_atg::text),enabled=false;
ALTER TABLE cooperation_purchase_quotes DROP COLUMN price_usd, DROP COLUMN paid_atg, DROP COLUMN gift_atg;
ALTER TABLE cooperation_purchase_quotes ADD CONSTRAINT purchase_gift_tiers_array CHECK(gift_tiers IS NULL OR (jsonb_typeof(gift_tiers)='array' AND jsonb_array_length(gift_tiers)>0));
CREATE OR REPLACE FUNCTION cooperation_preserve_purchase_quote() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'purchase quote facts cannot be deleted' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-'enabled') IS DISTINCT FROM (to_jsonb(OLD)-'enabled') THEN
  RAISE EXCEPTION 'purchase quote commercial facts are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cooperation_purchase_quote_immutable BEFORE UPDATE OR DELETE ON cooperation_purchase_quotes FOR EACH ROW EXECUTE FUNCTION cooperation_preserve_purchase_quote();
ALTER TABLE cooperation_purchase_orders ADD COLUMN pricing_snapshot JSONB;
CREATE OR REPLACE FUNCTION cooperation_preserve_purchase_order_terms() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.supplier_id,NEW.buyer_id,NEW.quote_id,NEW.due_usd,NEW.paid_atg,NEW.gift_atg,NEW.exit_terms,NEW.terms_reference,NEW.name,NEW.terms_summary,NEW.contract_attachment_id,NEW.quote_version,NEW.pricing_snapshot) IS DISTINCT FROM ROW(OLD.supplier_id,OLD.buyer_id,OLD.quote_id,OLD.due_usd,OLD.paid_atg,OLD.gift_atg,OLD.exit_terms,OLD.terms_reference,OLD.name,OLD.terms_summary,OLD.contract_attachment_id,OLD.quote_version,OLD.pricing_snapshot) THEN
  RAISE EXCEPTION 'purchase order accepted terms are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
ALTER TABLE cooperation_replenishment_authorizations ADD COLUMN amount_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(amount_usd>=0);
UPDATE cooperation_replenishment_authorizations SET enabled=false;
CREATE INDEX cooperation_purchase_quotes_supplier_page ON cooperation_purchase_quotes(supplier_id,created_at DESC,id);
CREATE INDEX cooperation_purchase_quotes_buyer_page ON cooperation_purchase_quotes(buyer_id,created_at DESC,id);
CREATE INDEX cooperation_purchase_orders_supplier_page ON cooperation_purchase_orders(supplier_id,created_at DESC,id);
CREATE INDEX cooperation_purchase_orders_buyer_page ON cooperation_purchase_orders(buyer_id,created_at DESC,id);
