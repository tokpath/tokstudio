CREATE INDEX organization_staff_setting_history ON organization_events(operator_id,(payload->>'StaffID'),created_at DESC,id DESC)
WHERE event_type='staff.membership_changed';
