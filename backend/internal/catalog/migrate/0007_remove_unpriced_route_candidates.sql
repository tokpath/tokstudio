-- Prelaunch route candidates whose upstream model has no usable cost cannot run.
-- Keep the route group so an administrator can select a priced replacement.
DELETE FROM catalog_route_candidates rc
USING catalog_route_groups rg, catalog_public_models m
WHERE rc.route_group_id = rg.id AND rg.public_model_id = m.id
  AND NOT EXISTS (
    SELECT 1
    FROM catalog_provider_model_mappings mp
    JOIN catalog_provider_models pm ON pm.provider_id = mp.provider_id
      AND pm.upstream_model_id = mp.upstream_model_id
    WHERE mp.public_model_id = m.id AND mp.provider_id = rc.provider_id
      AND mp.status = 'active' AND mp.upstream_model_id <> ''
      AND CASE COALESCE(m.capabilities_json->>'kind', 'text')
        WHEN 'embedding' THEN COALESCE(pm.unit_costs_json->>'input', '') <> ''
        WHEN 'image' THEN COALESCE(pm.unit_costs_json->>'image_count', '') <> ''
        WHEN 'video' THEN COALESCE(pm.unit_costs_json->>'video_second', '') <> ''
        WHEN 'audio' THEN COALESCE(pm.unit_costs_json->>'audio_second', '') <> ''
        ELSE COALESCE(pm.unit_costs_json->>'input', '') <> ''
          AND COALESCE(pm.unit_costs_json->>'output', '') <> ''
      END
  );
