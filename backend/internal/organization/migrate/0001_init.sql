-- W01 is independent of the legacy identity schema. UUIDs are supplied by the application.
CREATE TABLE organization_brands (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('official','oem')),
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    owner_operator_id UUID NOT NULL UNIQUE,
    api_base_url TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX organization_one_official_brand ON organization_brands(kind) WHERE kind='official';
CREATE TABLE organization_operators (
    id UUID PRIMARY KEY,
    brand_id UUID NOT NULL REFERENCES organization_brands(id) DEFERRABLE INITIALLY DEFERRED,
    parent_operator_id UUID REFERENCES organization_operators(id),
    kind TEXT NOT NULL CHECK (kind IN ('platform','oem','channel')),
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    legal_name TEXT NOT NULL DEFAULT '',
    lifecycle TEXT NOT NULL DEFAULT 'draft' CHECK (lifecycle IN ('draft','active','exiting','terminated')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(id,brand_id),
    CHECK ((kind='platform' AND parent_operator_id IS NULL) OR (kind<>'platform' AND parent_operator_id IS NOT NULL)),
    CHECK (id IS DISTINCT FROM parent_operator_id)
);
CREATE UNIQUE INDEX organization_one_platform ON organization_operators(kind) WHERE kind='platform';
ALTER TABLE organization_brands ADD CONSTRAINT organization_brand_owner_fk FOREIGN KEY(owner_operator_id) REFERENCES organization_operators(id) DEFERRABLE INITIALLY DEFERRED;
CREATE FUNCTION organization_validate_operator() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b organization_brands; p organization_operators;
BEGIN
    SELECT * INTO b FROM organization_brands WHERE id=NEW.brand_id;
    IF NEW.kind='platform' THEN
        IF b.kind<>'official' OR b.owner_operator_id<>NEW.id THEN RAISE EXCEPTION 'invalid platform brand' USING ERRCODE='23514'; END IF;
    ELSIF NEW.kind='oem' THEN
        SELECT * INTO p FROM organization_operators WHERE id=NEW.parent_operator_id;
        IF p.kind<>'platform' OR b.kind<>'oem' OR b.owner_operator_id<>NEW.id THEN RAISE EXCEPTION 'invalid OEM parent or brand' USING ERRCODE='23514'; END IF;
    ELSE
        SELECT * INTO p FROM organization_operators WHERE id=NEW.parent_operator_id;
        IF p.kind NOT IN ('platform','oem') OR p.brand_id<>NEW.brand_id OR b.owner_operator_id<>p.id THEN RAISE EXCEPTION 'invalid channel parent or brand' USING ERRCODE='23514'; END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER organization_operator_boundary AFTER INSERT OR UPDATE ON organization_operators DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION organization_validate_operator();
CREATE FUNCTION organization_validate_brand() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o organization_operators;
BEGIN
    SELECT * INTO o FROM organization_operators WHERE id=NEW.owner_operator_id;
    IF o.brand_id<>NEW.id OR (NEW.kind='official' AND o.kind<>'platform') OR (NEW.kind='oem' AND o.kind<>'oem') THEN RAISE EXCEPTION 'invalid brand owner' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER organization_brand_boundary AFTER INSERT OR UPDATE ON organization_brands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION organization_validate_brand();
CREATE TABLE organization_supply_agreements (
    id UUID PRIMARY KEY,
    supplier_id UUID NOT NULL REFERENCES organization_operators(id),
    buyer_id UUID NOT NULL UNIQUE REFERENCES organization_operators(id),
    status TEXT NOT NULL CHECK (status IN ('draft','active','paused','exited')),
    quote_reference TEXT NOT NULL DEFAULT '',
    exit_terms_reference TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (supplier_id<>buyer_id)
);
CREATE FUNCTION organization_validate_supply() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE b organization_operators; s organization_operators;
BEGIN
    SELECT * INTO b FROM organization_operators WHERE id=NEW.buyer_id;
    SELECT * INTO s FROM organization_operators WHERE id=NEW.supplier_id;
    IF b.parent_operator_id IS DISTINCT FROM s.id OR NOT ((s.kind='platform' AND b.kind IN ('oem','channel')) OR (s.kind='oem' AND b.kind='channel' AND s.brand_id=b.brand_id)) THEN RAISE EXCEPTION 'invalid supply relationship' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER organization_supply_boundary BEFORE INSERT OR UPDATE ON organization_supply_agreements FOR EACH ROW EXECUTE FUNCTION organization_validate_supply();
CREATE TABLE organization_operator_statuses (
    operator_id UUID PRIMARY KEY REFERENCES organization_operators(id),
    registration BOOLEAN NOT NULL DEFAULT false,
    new_purchase BOOLEAN NOT NULL DEFAULT false,
    renewal BOOLEAN NOT NULL DEFAULT false,
    model_usage BOOLEAN NOT NULL DEFAULT false,
    activity BOOLEAN NOT NULL DEFAULT false,
    settlement BOOLEAN NOT NULL DEFAULT true,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version>0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE organization_staff_memberships (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    staff_id UUID NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('owner','operations','finance','support','readonly','custom')),
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(operator_id,staff_id)
);
CREATE TABLE organization_staff_grants (
    membership_id UUID NOT NULL REFERENCES organization_staff_memberships(id),
    action TEXT NOT NULL CHECK(length(action)>0),
    PRIMARY KEY(membership_id,action)
);
CREATE TABLE organization_staff_invitations (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    email_normalized TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('owner','operations','finance','support','readonly')),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_by UUID,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK((accepted_at IS NULL)=(accepted_by IS NULL))
);
CREATE TABLE organization_customers (
    id UUID PRIMARY KEY,
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    display_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','closing','anonymized','suspended')),
    registered_at TIMESTAMPTZ NOT NULL,
    new_customer_recorded_at TIMESTAMPTZ NOT NULL,
    UNIQUE(id,brand_id)
);
CREATE TABLE organization_customer_identities (
    id UUID PRIMARY KEY,
    brand_id UUID NOT NULL,
    customer_id UUID NOT NULL,
    email_normalized TEXT NOT NULL CHECK(email_normalized=lower(trim(email_normalized))),
    is_current BOOLEAN NOT NULL,
    verified_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY(customer_id,brand_id) REFERENCES organization_customers(id,brand_id),
    CONSTRAINT organization_brand_email_unique UNIQUE(brand_id,email_normalized)
);
CREATE UNIQUE INDEX organization_one_current_email ON organization_customer_identities(customer_id) WHERE is_current;
CREATE FUNCTION organization_preserve_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'identity history cannot be deleted' USING ERRCODE='23514'; END IF;
    IF NEW.customer_id<>OLD.customer_id OR NEW.brand_id<>OLD.brand_id OR NEW.email_normalized<>OLD.email_normalized THEN RAISE EXCEPTION 'identity history cannot be reassigned' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER organization_identity_history BEFORE UPDATE OR DELETE ON organization_customer_identities FOR EACH ROW EXECUTE FUNCTION organization_preserve_identity();
CREATE TABLE organization_service_ownerships (
    id UUID PRIMARY KEY,
    customer_id UUID NOT NULL,
    brand_id UUID NOT NULL,
    operator_id UUID NOT NULL,
    version BIGINT NOT NULL CHECK(version>0),
    effective_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    evidence_reference TEXT NOT NULL,
    FOREIGN KEY(customer_id,brand_id) REFERENCES organization_customers(id,brand_id),
    FOREIGN KEY(operator_id,brand_id) REFERENCES organization_operators(id,brand_id),
    UNIQUE(customer_id,version),
    CHECK(ended_at IS NULL OR ended_at>=effective_at)
);
CREATE UNIQUE INDEX organization_one_current_ownership ON organization_service_ownerships(customer_id) WHERE ended_at IS NULL;
CREATE INDEX organization_current_operator_customers ON organization_service_ownerships(operator_id,customer_id) WHERE ended_at IS NULL;
CREATE TABLE organization_invite_codes (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE CHECK(code=upper(trim(code))),
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    kind TEXT NOT NULL CHECK(kind IN ('user','channel')),
    customer_id UUID REFERENCES organization_customers(id),
    operator_id UUID REFERENCES organization_operators(id),
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK((kind='user' AND customer_id IS NOT NULL AND operator_id IS NULL) OR (kind='channel' AND customer_id IS NULL AND operator_id IS NOT NULL))
);
CREATE UNIQUE INDEX organization_one_user_code ON organization_invite_codes(customer_id) WHERE kind='user';
CREATE UNIQUE INDEX organization_one_channel_code ON organization_invite_codes(operator_id) WHERE kind='channel';
CREATE FUNCTION organization_validate_code() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.kind='user' THEN
        IF NOT EXISTS(SELECT 1 FROM organization_customers WHERE id=NEW.customer_id AND brand_id=NEW.brand_id) THEN RAISE EXCEPTION 'invite brand mismatch' USING ERRCODE='23514'; END IF;
    ELSE
        IF NOT EXISTS(SELECT 1 FROM organization_operators WHERE id=NEW.operator_id AND brand_id=NEW.brand_id AND kind='channel') THEN RAISE EXCEPTION 'channel invite mismatch' USING ERRCODE='23514'; END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER organization_code_boundary BEFORE INSERT OR UPDATE ON organization_invite_codes FOR EACH ROW EXECUTE FUNCTION organization_validate_code();
CREATE TABLE organization_referral_edges (
    invitee_id UUID PRIMARY KEY REFERENCES organization_customers(id),
    inviter_id UUID NOT NULL REFERENCES organization_customers(id),
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    registration_operator_id UUID NOT NULL REFERENCES organization_operators(id),
    created_at TIMESTAMPTZ NOT NULL,
    CHECK(invitee_id<>inviter_id)
);
CREATE INDEX organization_inviter_children ON organization_referral_edges(inviter_id);
CREATE FUNCTION organization_validate_referral() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS(SELECT 1 FROM organization_service_ownerships a JOIN organization_service_ownerships b ON a.operator_id=b.operator_id AND a.brand_id=b.brand_id WHERE a.customer_id=NEW.invitee_id AND b.customer_id=NEW.inviter_id AND a.operator_id=NEW.registration_operator_id AND a.brand_id=NEW.brand_id AND a.ended_at IS NULL AND b.ended_at IS NULL) THEN RAISE EXCEPTION 'referrals cannot cross operators' USING ERRCODE='23514'; END IF;
    IF EXISTS(WITH RECURSIVE ancestors(id) AS (SELECT NEW.inviter_id UNION SELECT r.inviter_id FROM organization_referral_edges r JOIN ancestors a ON r.invitee_id=a.id) SELECT 1 FROM ancestors WHERE id=NEW.invitee_id) THEN RAISE EXCEPTION 'referral cycle' USING ERRCODE='23514'; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER organization_referral_boundary BEFORE INSERT ON organization_referral_edges FOR EACH ROW EXECUTE FUNCTION organization_validate_referral();
CREATE FUNCTION organization_immutable_referral() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'referral edges are immutable' USING ERRCODE='23514'; END $$;
CREATE TRIGGER organization_referral_immutable BEFORE UPDATE OR DELETE ON organization_referral_edges FOR EACH ROW EXECUTE FUNCTION organization_immutable_referral();
CREATE TABLE organization_registration_intents (
    id UUID PRIMARY KEY,
    brand_id UUID NOT NULL REFERENCES organization_brands(id),
    operator_id UUID NOT NULL REFERENCES organization_operators(id),
    email_normalized TEXT NOT NULL,
    invite_code_id UUID REFERENCES organization_invite_codes(id),
    inviter_id UUID REFERENCES organization_customers(id),
    verification_hash TEXT NOT NULL,
    purpose TEXT NOT NULL DEFAULT 'registration' CHECK(purpose IN ('registration','email_change','password_reset')),
    password_hash TEXT NOT NULL DEFAULT '',
    credential_version BIGINT NOT NULL DEFAULT 0,
    customer_id UUID REFERENCES organization_customers(id),
    status TEXT NOT NULL CHECK(status IN ('pending','verified','completed')),
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY(operator_id,brand_id) REFERENCES organization_operators(id,brand_id)
);
CREATE TABLE organization_customer_credentials (
    customer_id UUID PRIMARY KEY REFERENCES organization_customers(id),
    password_hash TEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE organization_access_tokens (
    id UUID PRIMARY KEY,
    customer_id UUID NOT NULL REFERENCES organization_customers(id),
    token_hash TEXT NOT NULL UNIQUE,
    credential_version BIGINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE organization_events (
    id UUID PRIMARY KEY,
    operator_id UUID REFERENCES organization_operators(id),
    actor_id UUID,
    event_type TEXT NOT NULL,
    evidence_reference TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL
);
