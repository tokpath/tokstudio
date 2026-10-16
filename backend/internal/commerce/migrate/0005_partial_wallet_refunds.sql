ALTER TABLE commerce_wallet_funding_allocations ADD COLUMN refunded_usd numeric(38,18) NOT NULL DEFAULT 0 CHECK(refunded_usd>=0 AND refunded_usd<=amount_usd);
UPDATE commerce_wallet_funding_allocations a SET refunded_usd=a.amount_usd FROM commerce_wallet_funding_commands c WHERE c.id=a.command_id AND c.kind='refund' AND a.status='consumed';
CREATE TABLE commerce_wallet_refund_observations(
 allocation_id uuid NOT NULL REFERENCES commerce_wallet_funding_allocations(id),fact_id text NOT NULL,amount_usd numeric(38,18) NOT NULL CHECK(amount_usd>0),created_at timestamptz NOT NULL,
 PRIMARY KEY(allocation_id,fact_id)
);
CREATE FUNCTION commerce_immutable_wallet_refund() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'wallet refund source evidence is immutable'; END $$;
CREATE TRIGGER commerce_wallet_refund_observations_immutable BEFORE UPDATE OR DELETE ON commerce_wallet_refund_observations FOR EACH ROW EXECUTE FUNCTION commerce_immutable_wallet_refund();
