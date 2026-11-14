ALTER TABLE incentives_withdrawals
  ADD COLUMN paid_currency text NOT NULL DEFAULT '',
  ADD COLUMN paid_original_amount numeric(38,18) NOT NULL DEFAULT 0 CHECK (paid_original_amount>=0);

UPDATE incentives_withdrawals w
SET paid_currency=e.payload->'proof'->>'currency',
    paid_original_amount=(e.payload->'proof'->>'original_amount')::numeric
FROM incentives_events e
WHERE w.status='paid' AND e.aggregate_id=w.id AND e.kind='withdrawal.payment_verified'
  AND e.payload->'proof'->>'currency' IS NOT NULL
  AND e.payload->'proof'->>'original_amount' IS NOT NULL;
