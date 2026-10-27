ALTER TABLE incentives_campaign_versions ADD COLUMN name TEXT NOT NULL DEFAULT '';
-- Empty activity names are displayed using the package name. Do not backfill
-- immutable published versions or disable their accounting protections.
ALTER TABLE incentives_campaign_versions DROP COLUMN gift_budget_atg, DROP COLUMN discount_budget_usd;
