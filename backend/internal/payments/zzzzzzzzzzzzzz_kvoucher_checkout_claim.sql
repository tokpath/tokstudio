ALTER TABLE payments_intents ADD COLUMN IF NOT EXISTS checkout_claimed_at timestamptz;
ALTER TABLE payments_intents ADD COLUMN IF NOT EXISTS checkout_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE payments_intents ADD COLUMN IF NOT EXISTS checkout_issue_until timestamptz;

CREATE TABLE IF NOT EXISTS payments_kvoucher_checkout_actions (
  merchant_id text NOT NULL,
  order_id text NOT NULL,
  action_key uuid NOT NULL,
  cart_hash text,
  PRIMARY KEY (order_id, action_key),
  FOREIGN KEY (merchant_id, order_id) REFERENCES payments_intents (merchant_id, order_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS payments_kvoucher_checkout_cart
  ON payments_kvoucher_checkout_actions (merchant_id, order_id, cart_hash)
  WHERE cart_hash IS NOT NULL;
