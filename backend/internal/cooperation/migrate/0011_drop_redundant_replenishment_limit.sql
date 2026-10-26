-- Every automatic purchase uses its explicitly authorized amount_usd.
ALTER TABLE cooperation_replenishment_authorizations DROP COLUMN single_limit_usd;
