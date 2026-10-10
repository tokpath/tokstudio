CREATE TABLE billing_quota_operations (
 id TEXT PRIMARY KEY,
 operation_id TEXT NOT NULL UNIQUE,
 actor_user_id TEXT NOT NULL,
 channel_org_id TEXT NOT NULL REFERENCES identity_channel_orgs(id),
 amount_minor BIGINT NOT NULL CHECK(amount_minor<>0),
 result_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
