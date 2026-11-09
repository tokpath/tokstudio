ALTER TABLE payment_method_versions ADD COLUMN method_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_method_versions ADD COLUMN paused BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE payment_method_identities (
 operator_id TEXT NOT NULL,
 kind TEXT NOT NULL,
 method_id TEXT NOT NULL,
 merchant_id TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(operator_id,kind),
 UNIQUE(operator_id,method_id)
);
