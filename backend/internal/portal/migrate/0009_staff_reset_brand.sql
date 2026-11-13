-- Retire outstanding staff password reset links issued through an OEM brand.
WITH expired AS (
 UPDATE portal_challenges ch SET expires_at = CURRENT_TIMESTAMP
 FROM portal_installation p
 WHERE ch.kind = 'staff_password_reset' AND ch.brand_id <> p.brand_id
 AND ch.completed_at IS NULL AND ch.expires_at > CURRENT_TIMESTAMP
 RETURNING ch.id
)
UPDATE portal_messages m SET expires_at = CURRENT_TIMESTAMP
FROM expired e WHERE m.subject_id = e.id AND m.status = 'pending' AND m.expires_at > CURRENT_TIMESTAMP;

-- Match first-owner links and their queued mail to the invitation's expiry.
WITH expired AS (
 UPDATE portal_challenges ch SET expires_at = i.expires_at
 FROM organization_staff_invitations i
 WHERE ch.intent_id = i.id AND i.role = 'owner' AND i.accepted_at IS NULL
 AND i.expires_at <= CURRENT_TIMESTAMP AND ch.expires_at > i.expires_at
 AND ch.kind IN ('staff_invitation', 'staff_invitation_manual') AND ch.completed_at IS NULL
 RETURNING ch.id, ch.expires_at
)
UPDATE portal_messages m SET expires_at = e.expires_at
FROM expired e WHERE m.subject_id = e.id AND m.status = 'pending' AND m.expires_at > e.expires_at;
