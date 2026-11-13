CREATE INDEX organization_pending_owner_invitations ON organization_staff_invitations(operator_id)
 WHERE role = 'owner' AND accepted_at IS NULL;

-- Outstanding links issued before the first owner joined cannot appoint a second one.
WITH first_owners AS (
 SELECT operator_id, min(created_at) AS joined_at
 FROM organization_staff_memberships WHERE role = 'owner' GROUP BY operator_id
)
UPDATE organization_staff_invitations i SET expires_at = CURRENT_TIMESTAMP
FROM first_owners o
WHERE i.operator_id = o.operator_id AND i.role = 'owner' AND i.accepted_at IS NULL
AND i.expires_at > CURRENT_TIMESTAMP AND i.created_at < o.joined_at;

-- Keep the most recent outstanding invitation where the first owner is still pending.
WITH pending AS (
 SELECT i.id, row_number() OVER (PARTITION BY i.operator_id ORDER BY i.created_at DESC, i.id DESC) AS position
 FROM organization_staff_invitations i
 WHERE i.role = 'owner' AND i.accepted_at IS NULL AND i.expires_at > CURRENT_TIMESTAMP
 AND NOT EXISTS (
  SELECT 1 FROM organization_staff_memberships m WHERE m.operator_id = i.operator_id AND m.role = 'owner'
 )
)
UPDATE organization_staff_invitations i SET expires_at = CURRENT_TIMESTAMP
FROM pending p WHERE i.id = p.id AND p.position > 1;
