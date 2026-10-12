ALTER TABLE ops_alerts ADD COLUMN scope_key TEXT NOT NULL DEFAULT '';
UPDATE ops_alerts SET scope_key=COALESCE(payload_json->>'provider_id',payload_json->>'key','');
-- Preserve historical records if the old unguarded evaluator opened duplicates.
WITH duplicates AS (
    SELECT id,ROW_NUMBER() OVER (PARTITION BY kind,scope_key ORDER BY created_at DESC,id DESC) AS rank
    FROM ops_alerts WHERE status='open'
)
UPDATE ops_alerts SET status='resolved',resolved_at=now()
WHERE id IN (SELECT id FROM duplicates WHERE rank>1);
CREATE UNIQUE INDEX ops_alert_open_scope_idx ON ops_alerts(kind,scope_key) WHERE status='open';
