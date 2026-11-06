-- Original receipt links remain immutable historical evidence. All active
-- invoice decisions use purchase orders and their server-owned paid amounts.
CREATE TABLE operations_invoice_orders (
 invoice_id UUID NOT NULL REFERENCES operations_invoices(id),
 order_id UUID NOT NULL,
 amount_money NUMERIC(38,18) NOT NULL CHECK(amount_money>0),
 PRIMARY KEY(invoice_id,order_id)
);
CREATE INDEX operations_invoice_orders_order ON operations_invoice_orders(order_id,invoice_id);
CREATE TRIGGER operations_invoice_orders_immutable BEFORE UPDATE OR DELETE ON operations_invoice_orders FOR EACH ROW EXECUTE FUNCTION operations_immutable_fact();

-- Keep the previous policy versions; record the new fixed purchase rule as a
-- new version instead of rewriting the history attached to issued documents.
UPDATE operations_invoice_policies
 SET stage='purchase',maximum_ratio=1,version=version+1,updated_at=clock_timestamp();
INSERT INTO operations_invoice_policy_versions(operator_id,party_kind,version,snapshot,created_at)
 SELECT p.operator_id,p.party_kind,p.version,to_jsonb(p),p.updated_at FROM operations_invoice_policies p;
