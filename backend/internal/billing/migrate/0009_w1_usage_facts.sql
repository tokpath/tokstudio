-- W1-①：usage 只存 TokenHub 已收到的透传事实。空 fact_source 表示未标定，禁止伪装成 live 上游。
ALTER TABLE billing_usage_events ADD COLUMN IF NOT EXISTS fact_source TEXT;
