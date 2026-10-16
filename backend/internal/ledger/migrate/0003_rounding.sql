CREATE TABLE ledger_rounding_results (
 account_id UUID NOT NULL REFERENCES ledger_accounts(id),
 source_key TEXT NOT NULL,
 operation_key TEXT NOT NULL,
 exact_numerator TEXT NOT NULL,
 exact_denominator TEXT NOT NULL,
 amount NUMERIC(38,18) NOT NULL CHECK(amount>=0),
 remainder_numerator TEXT NOT NULL,
 remainder_denominator TEXT NOT NULL,
 PRIMARY KEY(account_id,source_key,operation_key)
);
CREATE TRIGGER ledger_rounding_immutable BEFORE UPDATE OR DELETE ON ledger_rounding_results FOR EACH ROW EXECUTE FUNCTION ledger_immutable_fact();
