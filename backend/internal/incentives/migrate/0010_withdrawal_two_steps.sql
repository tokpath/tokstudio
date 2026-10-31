UPDATE incentives_withdrawals SET status = 'approved' WHERE status IN ('paying', 'unknown');

DO $$
DECLARE c RECORD;
BEGIN
  FOR c IN
    SELECT con.conname
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
    WHERE nsp.nspname = current_schema()
      AND rel.relname = 'incentives_withdrawals'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) LIKE '%paying%'
  LOOP
    EXECUTE format('ALTER TABLE incentives_withdrawals DROP CONSTRAINT %I', c.conname);
  END LOOP;
END $$;

ALTER TABLE incentives_withdrawals
  ADD CONSTRAINT incentives_withdrawals_status_check
  CHECK (status IN ('requested', 'approved', 'paid', 'cancelled', 'failed'));
