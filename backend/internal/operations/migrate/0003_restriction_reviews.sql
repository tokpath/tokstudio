CREATE TABLE operations_restriction_reviews (
 operator_id UUID NOT NULL,
 scope_kind TEXT NOT NULL CHECK(scope_kind IN ('brand','operator','customer')),
 scope_id UUID NOT NULL,
 action TEXT NOT NULL,
 freeze_version BIGINT NOT NULL CHECK(freeze_version>0),
 status TEXT NOT NULL CHECK(status IN ('warning','critical')),
 sequence BIGINT NOT NULL CHECK(sequence>0),
 last_notified_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(operator_id,scope_kind,scope_id,action)
);
