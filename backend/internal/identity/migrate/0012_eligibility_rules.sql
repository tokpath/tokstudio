-- 达线自动赋权：累计消费 / 单笔充值。A/B 用 platform；C（含其下 B）可用 channel 覆盖。
CREATE TABLE IF NOT EXISTS identity_eligibility_rules (
    id TEXT PRIMARY KEY,
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    spend_minor BIGINT NOT NULL DEFAULT 0,
    topup_minor BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_type, scope_id),
    CONSTRAINT identity_eligibility_nonneg CHECK (spend_minor >= 0 AND topup_minor >= 0)
);
