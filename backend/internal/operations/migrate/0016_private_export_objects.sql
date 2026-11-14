-- Previous CSV bodies are temporary and can be regenerated. Preserve the
-- request and download audit rows when removing them from PostgreSQL.
UPDATE operations_exports SET state='expired',lease_until=NULL
WHERE state IN ('pending','processing','ready');
ALTER TABLE operations_exports DROP COLUMN content_ciphertext,
    ADD COLUMN object_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN attempt_id UUID;
