-- Preserve code IDs, owners, enablement and all relationships. Only replace
-- long codes; existing six-character codes remain stable. No legacy aliases.
-- Match sign-up's lock order so an in-flight Google callback can finish
-- before codes and its pending state are updated together.
LOCK TABLE organization_google_states, organization_invite_codes IN EXCLUSIVE MODE;
DO $$
DECLARE
    item RECORD;
    candidate TEXT;
    alphabet CONSTANT TEXT := '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ';
    sample INTEGER;
    attempts INTEGER;
BEGIN
    FOR item IN SELECT id,code,brand_id FROM organization_invite_codes
                WHERE NOT (code COLLATE "C" ~ '^[0-9A-Z]{6}$') ORDER BY id FOR UPDATE
    LOOP
        attempts := 0;
        LOOP
            attempts := attempts + 1;
            IF attempts > 32 THEN RAISE EXCEPTION 'could not allocate a unique six-character invite code'; END IF;
            candidate := '';
            WHILE length(candidate) < 6 LOOP
                sample := get_byte(uuid_send(gen_random_uuid()), 0);
                -- Reject the last four byte values to avoid modulo bias.
                IF sample < 252 THEN candidate := candidate || substr(alphabet, sample % 36 + 1, 1); END IF;
            END LOOP;
            BEGIN
                UPDATE organization_invite_codes SET code=candidate WHERE id=item.id;
                EXIT;
            EXCEPTION WHEN unique_violation THEN
                -- Retry without losing this row or aborting the migration.
                NULL;
            END;
        END LOOP;
        -- A still-running Google sign-up keeps the same intended attribution.
        UPDATE organization_google_states SET invite_code=candidate
        WHERE brand_id=item.brand_id AND invite_code=item.code AND consumed_at IS NULL AND expires_at>now();
    END LOOP;
END $$;
ALTER TABLE organization_invite_codes ADD CONSTRAINT organization_invite_code_six_chars
CHECK (code COLLATE "C" ~ '^[0-9A-Z]{6}$');
