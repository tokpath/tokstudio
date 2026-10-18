-- Historical quote rows remain immutable. Existing reference-only terms are
-- preserved; new publications store the actual customer-facing terms.
ALTER TABLE commerce_package_quotes ADD COLUMN terms_summary TEXT NOT NULL DEFAULT '';
