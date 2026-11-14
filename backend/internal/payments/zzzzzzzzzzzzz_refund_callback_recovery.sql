ALTER TABLE payments_webhook_inbox ADD COLUMN IF NOT EXISTS exhausted_at timestamptz;
ALTER TABLE payments_webhook_inbox ADD COLUMN IF NOT EXISTS max_attempts integer NOT NULL DEFAULT 12;
CREATE INDEX IF NOT EXISTS payments_webhook_due ON payments_webhook_inbox(next_attempt_at, received_at)
    WHERE state='pending';
CREATE INDEX IF NOT EXISTS payments_refund_webhook_attention ON payments_webhook_inbox(exhausted_at DESC)
    WHERE exhausted_at IS NOT NULL AND state<>'processed';
