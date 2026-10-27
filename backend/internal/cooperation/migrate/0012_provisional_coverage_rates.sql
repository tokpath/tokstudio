-- Derived coverage observations preserve known commitments during incomplete
-- model configuration. This never changes procurement balances or cost basis.
CREATE TABLE cooperation_coverage_rates (
 operator_id UUID NOT NULL,
 beneficiary_id UUID NOT NULL,
 numerator TEXT NOT NULL CHECK(numerator ~ '^(0|[1-9][0-9]*)$'),
 denominator TEXT NOT NULL CHECK(denominator ~ '^[1-9][0-9]*$'),
 updated_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(operator_id,beneficiary_id)
);
