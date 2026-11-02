UPDATE operations_invoice_policies SET stage = 'purchase' WHERE stage = 'top_up';

DO $$
DECLARE c RECORD;
BEGIN
  FOR c IN
    SELECT con.conname
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
    WHERE nsp.nspname = current_schema()
      AND rel.relname = 'operations_invoice_policies'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) LIKE '%top_up%'
  LOOP
    EXECUTE format('ALTER TABLE operations_invoice_policies DROP CONSTRAINT %I', c.conname);
  END LOOP;
END $$;

ALTER TABLE operations_invoice_policies
  ADD CONSTRAINT operations_invoice_policies_stage_check
  CHECK (stage IN ('receipt', 'purchase', 'consumption'));
