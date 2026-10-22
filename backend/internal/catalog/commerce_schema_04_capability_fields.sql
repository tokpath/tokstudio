-- Public models keep the checked operation. Supplier and upstream capabilities
-- keep the call shape and the metering used to reserve and settle usage.
UPDATE tokenhub_public_models
SET definition = jsonb_set(definition, '{capabilities}', COALESCE((
  SELECT jsonb_agg(jsonb_build_object('operation', cap->>'operation'))
  FROM jsonb_array_elements(definition->'capabilities') AS cap
  WHERE jsonb_typeof(cap) = 'object' AND COALESCE(cap->>'operation', '') <> ''
), '[]'::jsonb))
WHERE jsonb_typeof(definition->'capabilities') = 'array';

UPDATE tokenhub_supplier_models
SET definition = jsonb_set(definition, '{capabilities}', COALESCE((
  SELECT jsonb_agg((
    SELECT COALESCE(jsonb_object_agg(key, value), '{}'::jsonb)
    FROM jsonb_each(cap)
    WHERE key IN ('operation', 'streaming', 'billable_bounds', 'usage_rules', 'subtract_paths')
  ))
  FROM jsonb_array_elements(definition->'capabilities') AS cap
  WHERE jsonb_typeof(cap) = 'object'
), '[]'::jsonb))
WHERE jsonb_typeof(definition->'capabilities') = 'array';

UPDATE tokenhub_upstreams
SET definition = jsonb_set(definition, '{capabilities}', COALESCE((
  SELECT jsonb_agg((
    SELECT COALESCE(jsonb_object_agg(key, value), '{}'::jsonb)
    FROM jsonb_each(cap)
    WHERE key IN ('operation', 'streaming', 'billable_bounds', 'usage_rules', 'subtract_paths')
  ))
  FROM jsonb_array_elements(definition->'capabilities') AS cap
  WHERE jsonb_typeof(cap) = 'object'
), '[]'::jsonb))
WHERE jsonb_typeof(definition->'capabilities') = 'array';

ALTER TABLE tokenhub_route_policies DROP COLUMN IF EXISTS evidence;
