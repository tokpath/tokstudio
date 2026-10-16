ALTER TABLE organization_customers DROP CONSTRAINT organization_customers_status_check;
ALTER TABLE organization_customers ADD CHECK(status IN ('active','stopped','closing','closed','anonymized','suspended'));
ALTER TABLE organization_customers ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK(version>0);
CREATE TABLE organization_scope_freezes (
 scope_kind TEXT NOT NULL CHECK(scope_kind IN ('brand','operator','customer')),
 scope_id UUID NOT NULL,action TEXT NOT NULL,
 enabled BOOLEAN NOT NULL,evidence TEXT NOT NULL,explanation TEXT NOT NULL,
 actor_id TEXT NOT NULL,review_due_at TIMESTAMPTZ,version BIGINT NOT NULL CHECK(version>0),updated_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(scope_kind,scope_id,action),CHECK(NOT enabled OR review_due_at IS NOT NULL)
);
CREATE TABLE organization_lifecycle_events (
 id UUID PRIMARY KEY,customer_id UUID NOT NULL REFERENCES organization_customers(id),operator_id UUID NOT NULL,
 action TEXT NOT NULL,actor_id TEXT NOT NULL,evidence TEXT NOT NULL,waiver_evidence TEXT NOT NULL,
 effective_at TIMESTAMPTZ NOT NULL
);
