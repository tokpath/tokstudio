ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_unit_check;
ALTER TABLE ledger_entries DROP CONSTRAINT ledger_entries_unit_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_unit_check CHECK(unit IN ('MONEY','ATG') OR (unit ~ '^[A-Z]{3}$' AND purpose IN ('control:external_cash','control:receipt_settlement')));
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_unit_check CHECK(unit IN ('MONEY','ATG') OR unit ~ '^[A-Z]{3}$');
