CREATE INDEX commerce_orders_reconciliation ON commerce_orders(original_operator_id);
CREATE INDEX commerce_receipt_allocations_order ON commerce_receipt_allocations(order_id);
CREATE INDEX commerce_unpublished_events ON commerce_events(created_at,id) WHERE published_at IS NULL;
