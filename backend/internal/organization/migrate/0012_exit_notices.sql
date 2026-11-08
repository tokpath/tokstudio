CREATE TABLE organization_takeover_locks (
 operator_id UUID PRIMARY KEY REFERENCES organization_operators(id),
 takeover_id UUID NOT NULL
);
CREATE TABLE organization_exit_notices (
 id UUID PRIMARY KEY, operator_id UUID NOT NULL UNIQUE REFERENCES organization_operators(id),
 parent_id UUID NOT NULL REFERENCES organization_operators(id),
 kind TEXT NOT NULL CHECK(kind IN ('oem','channel')),
 note TEXT NOT NULL DEFAULT '', actor_id UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL
);
