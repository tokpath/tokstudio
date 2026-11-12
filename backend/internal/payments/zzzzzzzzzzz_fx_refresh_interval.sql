ALTER TABLE payments_fx_settings ADD COLUMN IF NOT EXISTS refresh_interval_hours INTEGER NOT NULL DEFAULT 1 CHECK(refresh_interval_hours BETWEEN 1 AND 24);
