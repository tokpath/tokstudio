-- Version zero supplies the initial zero rates until a merchant publishes a rule.
-- Use the database clock as the latest possible effective time.
CREATE FUNCTION incentives_initialize_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO incentives_policies (
  id, operator_id, version, threshold_money, direct_rate, indirect_rate,
  confirmation_seconds, invitee_atg, inviter_atg, reward_validity_seconds, effective_at
 ) VALUES (gen_random_uuid(), NEW.id, 0, 0, 0, 0, 2592000, 0, 0, 2592000, LEAST(NEW.created_at, clock_timestamp()));
 RETURN NEW;
END;
$$;

CREATE TRIGGER incentives_initialize_policy AFTER INSERT ON organization_operators
 FOR EACH ROW EXECUTE FUNCTION incentives_initialize_policy();

INSERT INTO incentives_policies (
 id, operator_id, version, threshold_money, direct_rate, indirect_rate,
 confirmation_seconds, invitee_atg, inviter_atg, reward_validity_seconds, effective_at
)
SELECT gen_random_uuid(), op.id, 0, 0, 0, 0, 2592000, 0, 0, 2592000, LEAST(op.created_at, clock_timestamp())
FROM organization_operators op
WHERE NOT EXISTS (SELECT 1 FROM incentives_policies p WHERE p.operator_id = op.id);
