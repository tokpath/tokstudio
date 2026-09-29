-- A route group without candidates must not claim to be enabled.
UPDATE catalog_route_groups rg
SET status = 'inactive'
WHERE status = 'active'
  AND NOT EXISTS (
    SELECT 1 FROM catalog_route_candidates rc WHERE rc.route_group_id = rg.id
  );
