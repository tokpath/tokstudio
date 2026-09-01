-- 公开模型自己保存审核状态和创建人，手工创建（没有 mapping）也能进待审核队列。
ALTER TABLE catalog_public_models ADD COLUMN IF NOT EXISTS sync_state TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE catalog_public_models ADD COLUMN IF NOT EXISTS created_by_user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_public_models ADD COLUMN IF NOT EXISTS reviewed_by_user_id TEXT NOT NULL DEFAULT '';

UPDATE catalog_public_models
SET sync_state = 'published'
WHERE status IN ('published', 'deprecated')
  AND sync_state IN ('draft', '');

UPDATE catalog_provider_model_mappings
SET sync_state = 'published'
WHERE sync_state IN ('', 'draft')
  AND public_model_id IN (
    SELECT id FROM catalog_public_models WHERE status IN ('published', 'deprecated')
  );
