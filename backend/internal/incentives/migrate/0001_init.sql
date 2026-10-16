-- Depends on organization, ledger and commerce. All amounts use exact decimals.
CREATE SEQUENCE incentives_business_sequence;
CREATE TABLE incentives_policies (
 id UUID PRIMARY KEY,operator_id UUID NOT NULL REFERENCES organization_operators(id),version BIGINT NOT NULL,
 threshold_usd NUMERIC(38,18) NOT NULL CHECK(threshold_usd>=0),
 direct_rate NUMERIC(38,18) NOT NULL CHECK(direct_rate>=0 AND direct_rate<=1),
 indirect_rate NUMERIC(38,18) NOT NULL CHECK(indirect_rate>=0 AND indirect_rate<=1),
 confirmation_seconds BIGINT NOT NULL CHECK(confirmation_seconds>=0),
 invitee_atg NUMERIC(38,18) NOT NULL CHECK(invitee_atg>=0),inviter_atg NUMERIC(38,18) NOT NULL CHECK(inviter_atg>=0),
 reward_validity_seconds BIGINT NOT NULL CHECK(reward_validity_seconds>0),
 kol_cross_activity_limit INT NOT NULL DEFAULT 0 CHECK(kol_cross_activity_limit>=0),
 effective_at TIMESTAMPTZ NOT NULL,business_sequence BIGINT NOT NULL DEFAULT nextval('incentives_business_sequence'),
 UNIQUE(operator_id,version),CHECK(direct_rate+indirect_rate<=1)
);
CREATE FUNCTION incentives_immutable_fact() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'incentive facts are immutable; append a correction' USING ERRCODE='23514'; END $$;
CREATE TRIGGER incentives_policy_immutable BEFORE UPDATE OR DELETE ON incentives_policies FOR EACH ROW EXECUTE FUNCTION incentives_immutable_fact();
CREATE TABLE incentives_qualification_accounts (
 operator_id UUID NOT NULL REFERENCES organization_operators(id),customer_id UUID NOT NULL REFERENCES organization_customers(id),
 confirmed_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(confirmed_usd>=0),
 acquired_at TIMESTAMPTZ,acquired_policy_id UUID REFERENCES incentives_policies(id),
 pending_offset_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(pending_offset_usd>=0),
 PRIMARY KEY(operator_id,customer_id),CHECK((acquired_at IS NULL)=(acquired_policy_id IS NULL))
);
CREATE TABLE incentives_function_states (
 id UUID PRIMARY KEY,operator_id UUID NOT NULL,customer_id UUID NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('active','suspended','terminated')),
 release_blocked BOOLEAN NOT NULL,evidence_reference TEXT NOT NULL,actor_id TEXT NOT NULL,
 effective_at TIMESTAMPTZ NOT NULL,business_sequence BIGINT NOT NULL DEFAULT nextval('incentives_business_sequence')
);
CREATE FUNCTION incentives_preserve_qualification() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.acquired_at IS NOT NULL AND (NEW.acquired_at IS DISTINCT FROM OLD.acquired_at OR NEW.acquired_policy_id IS DISTINCT FROM OLD.acquired_policy_id) THEN RAISE EXCEPTION 'formal qualification history cannot be revoked' USING ERRCODE='23514'; END IF; RETURN NEW; END $$;
CREATE TRIGGER incentives_qualification_history BEFORE UPDATE ON incentives_qualification_accounts FOR EACH ROW EXECUTE FUNCTION incentives_preserve_qualification();
CREATE INDEX incentives_function_history ON incentives_function_states(operator_id,customer_id,effective_at,business_sequence);
CREATE TRIGGER incentives_function_immutable BEFORE UPDATE OR DELETE ON incentives_function_states FOR EACH ROW EXECUTE FUNCTION incentives_immutable_fact();
CREATE TABLE incentives_campaigns (
 id UUID PRIMARY KEY,operator_id UUID NOT NULL REFERENCES organization_operators(id),kind TEXT NOT NULL CHECK(kind IN ('merchant','kol')),
 owner_customer_id UUID REFERENCES organization_customers(id),current_version BIGINT NOT NULL,
 reserved_gift_atg NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(reserved_gift_atg>=0),used_gift_atg NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(used_gift_atg>=0),
 reserved_discount_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(reserved_discount_usd>=0),used_discount_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(used_discount_usd>=0),
 reserved_uses BIGINT NOT NULL DEFAULT 0 CHECK(reserved_uses>=0),used_uses BIGINT NOT NULL DEFAULT 0 CHECK(used_uses>=0),
 CHECK((kind='kol')=(owner_customer_id IS NOT NULL))
);
CREATE TABLE incentives_campaign_versions (
 id UUID PRIMARY KEY,campaign_id UUID NOT NULL REFERENCES incentives_campaigns(id),version BIGINT NOT NULL,
 package_id UUID NOT NULL REFERENCES commerce_packages(id),enabled BOOLEAN NOT NULL,
 starts_at TIMESTAMPTZ NOT NULL,ends_at TIMESTAMPTZ NOT NULL,
 eligibility TEXT NOT NULL CHECK(eligibility IN ('any','first_order','existing')),
 final_price_usd NUMERIC(38,18) CHECK(final_price_usd>0),gift_atg NUMERIC(38,18) NOT NULL CHECK(gift_atg>=0),
 concession_rate NUMERIC(38,18) NOT NULL CHECK(concession_rate>=0 AND concession_rate<=1),
 per_customer_limit BIGINT NOT NULL CHECK(per_customer_limit>0),total_use_limit BIGINT NOT NULL CHECK(total_use_limit>=0),
 gift_budget_atg NUMERIC(38,18) NOT NULL CHECK(gift_budget_atg>=0),discount_budget_usd NUMERIC(38,18) NOT NULL CHECK(discount_budget_usd>=0),
 created_at TIMESTAMPTZ NOT NULL,UNIQUE(campaign_id,version),CHECK(ends_at>starts_at)
);
CREATE TRIGGER incentives_campaign_version_immutable BEFORE UPDATE OR DELETE ON incentives_campaign_versions FOR EACH ROW EXECUTE FUNCTION incentives_immutable_fact();
CREATE TABLE incentives_combinations (operator_id UUID NOT NULL,merchant_campaign_id UUID NOT NULL REFERENCES incentives_campaigns(id),kol_campaign_id UUID NOT NULL REFERENCES incentives_campaigns(id),PRIMARY KEY(merchant_campaign_id,kol_campaign_id));
CREATE TABLE incentives_campaign_customer_counts (campaign_id UUID NOT NULL REFERENCES incentives_campaigns(id),customer_id UUID NOT NULL,reserved BIGINT NOT NULL DEFAULT 0 CHECK(reserved>=0),used BIGINT NOT NULL DEFAULT 0 CHECK(used>=0),PRIMARY KEY(campaign_id,customer_id));
CREATE TABLE incentives_kol_customer_counts (operator_id UUID NOT NULL,kol_customer_id UUID NOT NULL,customer_id UUID NOT NULL,reserved BIGINT NOT NULL DEFAULT 0 CHECK(reserved>=0),used BIGINT NOT NULL DEFAULT 0 CHECK(used>=0),PRIMARY KEY(operator_id,kol_customer_id,customer_id));
CREATE TABLE incentives_order_snapshots (
 order_id UUID PRIMARY KEY REFERENCES commerce_orders(id) DEFERRABLE INITIALLY DEFERRED,
 operator_id UUID NOT NULL,customer_id UUID NOT NULL,policy_id UUID NOT NULL REFERENCES incentives_policies(id),
 direct_customer_id UUID,indirect_customer_id UUID,direct_rate NUMERIC(38,18) NOT NULL,indirect_rate NUMERIC(38,18) NOT NULL,concession_rate NUMERIC(38,18) NOT NULL,
 merchant_campaign_id UUID REFERENCES incentives_campaigns(id),kol_campaign_id UUID REFERENCES incentives_campaigns(id),
 merchant_version_id UUID REFERENCES incentives_campaign_versions(id),kol_version_id UUID REFERENCES incentives_campaign_versions(id),
 kol_customer_id UUID,merchant_gift_atg NUMERIC(38,18) NOT NULL,kol_gift_atg NUMERIC(38,18) NOT NULL,discount_usd NUMERIC(38,18) NOT NULL,
 gift_remainder TEXT NOT NULL DEFAULT '0',status TEXT NOT NULL CHECK(status IN ('reserved','fulfilled','released')),created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE incentives_confirmation_events (
 id UUID PRIMARY KEY,source_event_id UUID NOT NULL UNIQUE,operator_id UUID NOT NULL,customer_id UUID NOT NULL,order_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('usage','expiry','correction')),paid_usd NUMERIC(38,18) NOT NULL,
 policy_id UUID NOT NULL REFERENCES incentives_policies(id),occurred_at TIMESTAMPTZ NOT NULL,source_occurred_at TIMESTAMPTZ NOT NULL,confirm_at TIMESTAMPTZ NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','disputed','confirmed')),fingerprint TEXT NOT NULL,
 business_sequence BIGINT NOT NULL DEFAULT nextval('incentives_business_sequence'),
 original_event_id UUID REFERENCES incentives_confirmation_events(id),
 CHECK((kind='correction' AND paid_usd<0 AND original_event_id IS NOT NULL) OR (kind IN ('usage','expiry') AND paid_usd>0 AND original_event_id IS NULL))
);
CREATE INDEX incentives_due_events ON incentives_confirmation_events(operator_id,confirm_at,business_sequence) WHERE status='pending';
CREATE FUNCTION incentives_preserve_confirmation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF TG_OP='DELETE' THEN RAISE EXCEPTION 'confirmation facts cannot be deleted' USING ERRCODE='23514'; END IF; IF (to_jsonb(NEW)-'status') IS DISTINCT FROM (to_jsonb(OLD)-'status') THEN RAISE EXCEPTION 'confirmation economics are immutable' USING ERRCODE='23514'; END IF; RETURN NEW; END $$;
CREATE TRIGGER incentives_confirmation_history BEFORE UPDATE OR DELETE ON incentives_confirmation_events FOR EACH ROW EXECUTE FUNCTION incentives_preserve_confirmation();
CREATE TABLE incentives_commission_entries (
 id UUID PRIMARY KEY,event_id UUID NOT NULL REFERENCES incentives_confirmation_events(id),operator_id UUID NOT NULL,order_id UUID NOT NULL,beneficiary_id UUID NOT NULL,
 hop INT NOT NULL CHECK(hop IN (1,2)),amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),reversed_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(reversed_usd>=0),
 offset_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(offset_usd>=0),status TEXT NOT NULL CHECK(status IN ('frozen','available','reversed')),
 confirm_at TIMESTAMPTZ NOT NULL,created_at TIMESTAMPTZ NOT NULL,UNIQUE(event_id,hop),CHECK(reversed_usd<=amount_usd),CHECK(offset_usd<=amount_usd)
);
CREATE INDEX incentives_frozen_commissions ON incentives_commission_entries(operator_id,beneficiary_id,confirm_at) WHERE status='frozen';
CREATE TABLE incentives_commission_carries (operator_id UUID NOT NULL,order_id UUID NOT NULL,beneficiary_id UUID NOT NULL,hop INT NOT NULL,base_usd NUMERIC(38,18) NOT NULL CHECK(base_usd>=0),credited_usd NUMERIC(38,18) NOT NULL CHECK(credited_usd>=0),reversed_usd NUMERIC(38,18) NOT NULL CHECK(reversed_usd>=0 AND reversed_usd<=credited_usd),rate NUMERIC(38,18) NOT NULL CHECK(rate>=0 AND rate<=1),remainder_rat TEXT NOT NULL,PRIMARY KEY(order_id,beneficiary_id,hop));
CREATE TABLE incentives_commission_bases (event_id UUID NOT NULL REFERENCES incentives_confirmation_events(id),order_id UUID NOT NULL,beneficiary_id UUID NOT NULL,hop INT NOT NULL,base_usd NUMERIC(38,18) NOT NULL CHECK(base_usd>0),PRIMARY KEY(event_id,hop));
CREATE TABLE incentives_corrections (id UUID PRIMARY KEY,operator_id UUID NOT NULL,original_event_id UUID NOT NULL REFERENCES incentives_confirmation_events(id),business_key TEXT NOT NULL,amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),evidence TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL,UNIQUE(operator_id,business_key));
CREATE TABLE incentives_offsets (id UUID PRIMARY KEY,operator_id UUID NOT NULL,customer_id UUID NOT NULL,source_reference TEXT NOT NULL,amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),recovered_usd NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(recovered_usd>=0 AND recovered_usd<=amount_usd),created_at TIMESTAMPTZ NOT NULL);
CREATE TABLE incentives_withdrawal_policies (id UUID PRIMARY KEY,operator_id UUID NOT NULL,version BIGINT NOT NULL,minimum_usd NUMERIC(38,18) NOT NULL CHECK(minimum_usd>0),flat_fee_usd NUMERIC(38,18) NOT NULL CHECK(flat_fee_usd>=0),fee_rate NUMERIC(38,18) NOT NULL CHECK(fee_rate>=0 AND fee_rate<1),requires_dual_approval BOOLEAN NOT NULL,method_reference TEXT NOT NULL,effective_at TIMESTAMPTZ NOT NULL,UNIQUE(operator_id,version));
CREATE TRIGGER incentives_withdrawal_policy_immutable BEFORE UPDATE OR DELETE ON incentives_withdrawal_policies FOR EACH ROW EXECUTE FUNCTION incentives_immutable_fact();
CREATE TABLE incentives_withdrawals (
 id UUID PRIMARY KEY,operator_id UUID NOT NULL,customer_id UUID NOT NULL,business_key TEXT NOT NULL,fingerprint TEXT NOT NULL,
 policy_id UUID NOT NULL REFERENCES incentives_withdrawal_policies(id),amount_usd NUMERIC(38,18) NOT NULL CHECK(amount_usd>0),fee_usd NUMERIC(38,18) NOT NULL CHECK(fee_usd>=0),net_usd NUMERIC(38,18) NOT NULL CHECK(net_usd>0),fee_remainder TEXT NOT NULL,
 recipient_reference TEXT NOT NULL,hold_id UUID NOT NULL REFERENCES ledger_holds(id),status TEXT NOT NULL CHECK(status IN ('requested','approved','paying','unknown','paid','cancelled','failed')),
 approved_by UUID,payment_actor_id UUID,payment_intent_id UUID,payment_reference TEXT,payment_evidence TEXT NOT NULL DEFAULT '',paid_at TIMESTAMPTZ,created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(operator_id,business_key),UNIQUE(operator_id,payment_reference),CHECK(amount_usd=fee_usd+net_usd)
);
CREATE TABLE incentives_registration_promises (
 intent_id UUID PRIMARY KEY,operator_id UUID NOT NULL,inviter_id UUID,policy_id UUID NOT NULL REFERENCES incentives_policies(id),
 invitee_atg NUMERIC(38,18) NOT NULL,inviter_atg NUMERIC(38,18) NOT NULL,reward_validity_seconds BIGINT NOT NULL,
 actual_inviter_atg NUMERIC(38,18) NOT NULL DEFAULT 0 CHECK(actual_inviter_atg>=0),
 coverage_reference TEXT NOT NULL,coverage_version BIGINT NOT NULL,expires_at TIMESTAMPTZ NOT NULL,status TEXT NOT NULL CHECK(status IN ('reserved','pending_delivery','delivered','released')),
 customer_id UUID,registered_at TIMESTAMPTZ,last_error TEXT NOT NULL DEFAULT '',created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE incentives_registration_deliveries (intent_id UUID NOT NULL REFERENCES incentives_registration_promises(intent_id),customer_id UUID NOT NULL,grant_id UUID NOT NULL REFERENCES ledger_grants(id),PRIMARY KEY(intent_id,customer_id));
CREATE TABLE incentives_events (id UUID PRIMARY KEY,operator_id UUID NOT NULL,aggregate_id UUID NOT NULL,kind TEXT NOT NULL,payload JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL,published_at TIMESTAMPTZ);
