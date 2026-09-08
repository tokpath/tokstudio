-- 渠道 v2：分佣资格；激励规则在 billing。
ALTER TABLE identity_users
    ADD COLUMN IF NOT EXISTS can_commission BOOLEAN NOT NULL DEFAULT false;
