CREATE TABLE incentives_withdrawal_settings (
 operator_id UUID PRIMARY KEY,
 minimum_money NUMERIC(38,18) NOT NULL CHECK(minimum_money>0)
);
INSERT INTO incentives_withdrawal_settings
SELECT DISTINCT ON(operator_id) operator_id,minimum_money
FROM incentives_withdrawal_policies ORDER BY operator_id,version DESC;

ALTER TABLE incentives_withdrawal_policies
 ADD COLUMN method_id UUID,
 ADD COLUMN method_name TEXT NOT NULL DEFAULT '人工转账',
 ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT true;
-- Existing versions describe the same initial method. Retain immutable
-- withdrawal snapshots; future fee changes create new method versions.
ALTER TABLE incentives_withdrawal_policies DISABLE TRIGGER incentives_withdrawal_policy_immutable;
UPDATE incentives_withdrawal_policies SET method_id=operator_id;
ALTER TABLE incentives_withdrawal_policies ENABLE TRIGGER incentives_withdrawal_policy_immutable;
ALTER TABLE incentives_withdrawal_policies ALTER COLUMN method_id SET NOT NULL;
CREATE INDEX incentives_withdrawal_method_versions ON incentives_withdrawal_policies(operator_id,method_id,version DESC);
ALTER TABLE incentives_withdrawals ADD COLUMN method_id UUID, ADD COLUMN method_name TEXT NOT NULL DEFAULT '';
UPDATE incentives_withdrawals w SET method_id=p.method_id,method_name=p.method_name FROM incentives_withdrawal_policies p WHERE p.id=w.policy_id;
