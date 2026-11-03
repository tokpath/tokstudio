-- A file saved before the invoice result is registered can be withdrawn.
-- Issued, rejected, and voided invoices keep their stored files.
DO $$
DECLARE c RECORD;
BEGIN
  FOR c IN
    SELECT con.conname
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
    WHERE nsp.nspname = current_schema()
      AND rel.relname = 'operations_invoice_files'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) LIKE '%uploading%'
  LOOP
    EXECUTE format('ALTER TABLE operations_invoice_files DROP CONSTRAINT %I', c.conname);
  END LOOP;
END $$;

ALTER TABLE operations_invoice_files
  ADD CONSTRAINT operations_invoice_files_state_check
  CHECK (state IN ('uploading', 'ready', 'removed'));
