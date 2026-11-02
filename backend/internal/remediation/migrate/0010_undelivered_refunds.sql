DO $$
DECLARE c RECORD;
BEGIN
  FOR c IN
    SELECT con.conname
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
    WHERE nsp.nspname = current_schema()
      AND rel.relname = 'remediation_cases'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) LIKE '%package_exception%'
  LOOP
    EXECUTE format('ALTER TABLE remediation_cases DROP CONSTRAINT %I', c.conname);
  END LOOP;
END $$;

ALTER TABLE remediation_cases
  ADD CONSTRAINT remediation_cases_kind_check
  CHECK (kind IN ('wallet', 'package_exception', 'unallocated', 'procurement_nondelivery'));
