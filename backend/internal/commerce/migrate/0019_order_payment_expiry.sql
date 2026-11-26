ALTER TABLE commerce_orders ADD COLUMN closed_for_payment_expiry boolean NOT NULL DEFAULT false;

CREATE INDEX commerce_order_expiry_events ON commerce_events (aggregate_id)
WHERE kind='order.closed'
  AND payload->>'evidence' IN ('付款期限已过且核实没有未完成的线上付款或到账记录，自动关闭订单',
                              '超过付款期限仍未记下到账，系统自动取消并释放占用');

-- Match the previous expiry worker's audit evidence, including orders later
-- transferred to another operator. Customer cancellations have separate evidence.
UPDATE commerce_orders o SET closed_for_payment_expiry=true
FROM commerce_events e
WHERE o.id=e.aggregate_id
  AND o.status='closed' AND o.external_allocated_money=0
  AND e.kind='order.closed'
  AND e.payload->>'evidence' IN ('付款期限已过且核实没有未完成的线上付款或到账记录，自动关闭订单',
                                '超过付款期限仍未记下到账，系统自动取消并释放占用');
