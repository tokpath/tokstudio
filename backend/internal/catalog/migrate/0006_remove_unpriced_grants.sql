-- Prelaunch grants without a channel settlement price cannot be used.
-- Imported OFOX models remain in the catalog and can be authorized explicitly.
DELETE FROM catalog_channel_model_policies
WHERE wholesale_json IS NULL OR wholesale_json = '{}'::jsonb;
