-- Temporary export artifacts are regenerated in the new CSV format. Preserve
-- the job and its access history; no business or financial facts are removed.
UPDATE operations_exports SET state='failed',error_code='format_changed',content_ciphertext='',lease_until=NULL WHERE state IN ('pending','processing','ready');
ALTER TABLE operations_exports DROP COLUMN owner_id;
