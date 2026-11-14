-- Older runs created duplicate open items for the same underlying finding.
DELETE FROM operations_discrepancies
WHERE id IN (
  SELECT id FROM (
    SELECT d.id, row_number() OVER (
      PARTITION BY d.operator_id, d.domain, d.kind, d.reference, d.unit
      ORDER BY r.created_at DESC, d.id DESC
    ) AS occurrence
    FROM operations_discrepancies d JOIN operations_reconciliations r ON r.id=d.run_id
    WHERE d.state='open'
  ) ranked WHERE occurrence>1
);
CREATE UNIQUE INDEX operations_discrepancy_open
    ON operations_discrepancies(operator_id,domain,kind,reference,unit) WHERE state='open';
CREATE INDEX operations_discrepancy_page ON operations_discrepancies(operator_id,state,id);
