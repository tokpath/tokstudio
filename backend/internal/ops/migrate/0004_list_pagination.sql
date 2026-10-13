CREATE INDEX IF NOT EXISTS ops_alerts_created_id_idx ON ops_alerts (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS ops_alerts_status_created_id_idx ON ops_alerts (status, created_at DESC, id DESC);
