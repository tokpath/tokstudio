ALTER TABLE commerce_orders ADD COLUMN monetary_snapshot JSONB NOT NULL DEFAULT '{}';
