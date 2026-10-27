ALTER TABLE incentives_campaign_versions ADD COLUMN name TEXT NOT NULL DEFAULT '';
UPDATE incentives_campaign_versions v SET name=p.name FROM commerce_packages p WHERE p.id=v.package_id;
ALTER TABLE incentives_campaign_versions DROP COLUMN gift_budget_atg, DROP COLUMN discount_budget_usd;
