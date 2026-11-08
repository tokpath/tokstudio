ALTER TABLE commerce_receipts ADD COLUMN payment_fee_money NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(payment_fee_money>=0 AND payment_fee_money<gross_money);
CREATE FUNCTION commerce_preserve_payment_fee() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.payment_fee_money IS DISTINCT FROM OLD.payment_fee_money AND NOT (OLD.payment_fee_money=0 AND OLD.allocated_money=0 AND NEW.allocated_money=0 AND OLD.provider='manual') THEN RAISE EXCEPTION 'receipt fee snapshot is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER commerce_payment_fee_immutable BEFORE UPDATE ON commerce_receipts FOR EACH ROW EXECUTE FUNCTION commerce_preserve_payment_fee();
DO $$ DECLARE f RECORD; definition TEXT; BEGIN
 FOR f IN SELECT p.oid FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN ('commerce_allocate_receipt','commerce_reserve_receipt_funds') LOOP
  definition:=pg_get_functiondef(f.oid);
  EXECUTE replace(definition,'allocated_money=allocated_money+NEW.amount_money','allocated_money=allocated_money+NEW.amount_money+r.payment_fee_money');
 END LOOP;
END $$;
