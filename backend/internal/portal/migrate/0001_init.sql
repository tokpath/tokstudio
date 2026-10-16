CREATE TABLE portal_staff (
 id uuid PRIMARY KEY, email text NOT NULL UNIQUE, display_name text NOT NULL,
 password_hash text NOT NULL, version bigint NOT NULL DEFAULT 1,
 status text NOT NULL CHECK(status IN ('active','disabled')),
 email_verified boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL
);
CREATE TABLE portal_installation (
 id text PRIMARY KEY CHECK(id='platform'), brand_id uuid NOT NULL,
 operator_id uuid NOT NULL, owner_staff_id uuid NOT NULL, created_at timestamptz NOT NULL
);
CREATE TABLE portal_sessions (
 token_hash text PRIMARY KEY, kind text NOT NULL CHECK(kind IN ('customer','staff')),
 brand_id uuid NOT NULL, subject_id uuid NOT NULL, credential_version bigint NOT NULL,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL
);
CREATE INDEX portal_session_expiry ON portal_sessions(expires_at);
CREATE TABLE portal_challenges (
 id uuid PRIMARY KEY, intent_id uuid NOT NULL, brand_id uuid NOT NULL,
 kind text NOT NULL, display_name text NOT NULL DEFAULT '',
 proof_hash text NOT NULL, expires_at timestamptz NOT NULL,
 completed_at timestamptz, created_at timestamptz NOT NULL
);
CREATE TABLE portal_messages (
 id uuid PRIMARY KEY, subject_id uuid NOT NULL, ciphertext text NOT NULL,
 status text NOT NULL DEFAULT 'pending', delivered_at timestamptz,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL
);
CREATE TABLE portal_commands (
 scope text NOT NULL, business_key text NOT NULL, fingerprint text NOT NULL,
 response jsonb NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(scope,business_key)
);
CREATE TABLE portal_order_quotes (
 id uuid PRIMARY KEY, customer_id uuid NOT NULL, operator_id uuid NOT NULL,
 package_id uuid NOT NULL, quote_id uuid NOT NULL, terms_version text NOT NULL,
 selection jsonb NOT NULL DEFAULT '{}', preview jsonb NOT NULL,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL
);
CREATE TABLE portal_order_checkout (
 order_id uuid PRIMARY KEY, customer_id uuid NOT NULL, merchant_id uuid,
 auto_renew boolean NOT NULL DEFAULT false, allow_commission boolean NOT NULL DEFAULT false,
 action jsonb, payment_order jsonb, created_at timestamptz NOT NULL
);
CREATE TABLE portal_security_attempts (
 key text PRIMARY KEY, window_start timestamptz NOT NULL, attempts int NOT NULL
);
CREATE TABLE portal_valuations (
 id uuid PRIMARY KEY, operator_id uuid NOT NULL, currency text NOT NULL,
 usd_per_unit numeric(38,18) NOT NULL CHECK(usd_per_unit>0), source text NOT NULL,
 actor_id uuid NOT NULL, effective_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL, CHECK(expires_at>effective_at)
);
CREATE INDEX portal_valuation_lookup ON portal_valuations(operator_id,currency,effective_at,expires_at);
CREATE TABLE portal_withdrawal_quotes (
 id uuid PRIMARY KEY, customer_id uuid NOT NULL, operator_id uuid NOT NULL,
 quote jsonb NOT NULL, expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL
);
CREATE TABLE portal_price_checks (
 price_id text NOT NULL, operator_id uuid NOT NULL, status text NOT NULL,
 snapshot jsonb, error_code text NOT NULL DEFAULT '', checked_at timestamptz NOT NULL,
 PRIMARY KEY(price_id,operator_id)
);
CREATE TABLE portal_schedules (
 id text PRIMARY KEY, kind text NOT NULL, payload jsonb NOT NULL,
 last_job_id text, next_due_at timestamptz NOT NULL, created_at timestamptz NOT NULL
);
CREATE TABLE portal_worker_progress (
 id text PRIMARY KEY, cursor text NOT NULL DEFAULT ''
);
CREATE TABLE portal_renewal_notices (
 subscription_id uuid NOT NULL, quote_id uuid NOT NULL, cycle_due_at timestamptz NOT NULL,
 notification_id uuid NOT NULL, accepted_at timestamptz,
 PRIMARY KEY(subscription_id,quote_id,cycle_due_at)
);

CREATE TABLE portal_procurement_checkouts(order_id uuid PRIMARY KEY,buyer_id uuid NOT NULL,actor_id uuid NOT NULL,merchant_id uuid NOT NULL,payment_order jsonb NOT NULL,created_at timestamptz NOT NULL);

CREATE TABLE portal_finance_evidence(id uuid PRIMARY KEY,operator_id uuid NOT NULL,business_key text NOT NULL,fingerprint text NOT NULL,object_key text NOT NULL UNIQUE,filename text NOT NULL,content_type text NOT NULL,size_bytes int NOT NULL,status text NOT NULL CHECK(status IN ('pending','ready')),created_by uuid NOT NULL,created_at timestamptz NOT NULL,UNIQUE(operator_id,business_key));
