-- USD cap window. lifetime keeps the running total; month, quarter and year
-- start again at the beginning of the current calendar period.
ALTER TABLE identity_api_keys ADD COLUMN budget_period TEXT NOT NULL DEFAULT 'lifetime' CHECK (budget_period IN ('lifetime','month','quarter','year'));
ALTER TABLE identity_api_keys ADD COLUMN budget_window_start TIMESTAMPTZ;
