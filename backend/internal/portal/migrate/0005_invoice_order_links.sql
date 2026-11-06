-- Resolve existing receipt-based documents once. Never invent a purchase
-- attribution when an old receipt funded multiple possible orders.
CREATE TEMP TABLE invoice_order_candidates ON COMMIT DROP AS
 SELECT DISTINCT i.id AS invoice_id,s.receipt_id,o.id AS order_id
 FROM operations_invoices i JOIN operations_invoice_sources s ON s.invoice_id=i.id
 JOIN commerce_receipt_allocations a ON a.receipt_id=s.receipt_id
 JOIN commerce_orders o ON o.id=a.order_id
 WHERE i.party_kind='customer' AND o.customer_id=i.customer_id AND o.original_operator_id=i.operator_id
 AND o.kind IN ('package','renewal') AND o.created_at<=i.created_at
 UNION
 SELECT DISTINCT i.id,s.receipt_id,o.id
 FROM operations_invoices i JOIN operations_invoice_sources s ON s.invoice_id=i.id
 JOIN commerce_wallet_funding_lots lot ON lot.receipt_id=s.receipt_id
 JOIN commerce_wallet_funding_allocations a ON a.lot_id=lot.id
 JOIN commerce_wallet_funding_commands cmd ON cmd.id=a.command_id AND cmd.kind='order'
 JOIN commerce_orders o ON o.id=cmd.reference_id
 WHERE i.party_kind='customer' AND o.customer_id=i.customer_id AND o.original_operator_id=i.operator_id
 AND a.status='consumed' AND o.kind IN ('package','renewal') AND o.created_at<=i.created_at
 UNION
 SELECT DISTINCT i.id,s.receipt_id,o.id
 FROM operations_invoices i JOIN operations_invoice_sources s ON s.invoice_id=i.id
 JOIN cooperation_purchase_allocations a ON a.receipt_id=s.receipt_id
 JOIN cooperation_purchase_orders o ON o.id=a.order_id
 WHERE i.party_kind='procurement' AND o.buyer_id=i.buyer_operator_id AND o.supplier_id=i.operator_id AND o.created_at<=i.created_at;
DO $$ BEGIN
 IF EXISTS (
  SELECT s.invoice_id,s.receipt_id FROM operations_invoice_sources s
  LEFT JOIN invoice_order_candidates c ON c.invoice_id=s.invoice_id AND c.receipt_id=s.receipt_id
  GROUP BY s.invoice_id,s.receipt_id HAVING count(c.order_id)<>1
 ) THEN
  RAISE EXCEPTION 'Existing invoices have ambiguous purchase attribution; preserve the records and verify the original purchases before migrating';
 END IF;
END $$;
INSERT INTO operations_invoice_orders(invoice_id,order_id,amount_money)
 SELECT s.invoice_id,c.order_id,sum(s.amount_money)
 FROM operations_invoice_sources s JOIN invoice_order_candidates c ON c.invoice_id=s.invoice_id AND c.receipt_id=s.receipt_id
 GROUP BY s.invoice_id,c.order_id;
