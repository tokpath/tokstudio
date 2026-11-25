SELECT version FROM tokenhub_catalog_revisions WHERE id='global' FOR UPDATE;

WITH affected_models AS (
    SELECT s.id, jsonb_path_query_array(s.definition, '$.capabilities[*] ? (@.operation == "chat" || @.operation == "responses")') AS text_capabilities
    FROM tokenhub_supplier_models s
    JOIN tokenhub_provider_credentials c ON c.id=s.definition->>'credential_id' AND c.subscription_kind<>''
    WHERE s.definition @? '$.capabilities[*] ? (@.operation == "image_generation" || @.operation == "image_edit" || @.operation == "video_generation")'
), removed_models AS (
    DELETE FROM tokenhub_supplier_models s USING affected_models m
    WHERE s.id=m.id AND m.text_capabilities='[]'::jsonb
    RETURNING s.id
), trimmed_models AS (
    UPDATE tokenhub_supplier_models s
    SET definition=s.definition || jsonb_build_object('capabilities',m.text_capabilities,'version',s.version+1), version=s.version+1
    FROM affected_models m WHERE s.id=m.id AND m.text_capabilities<>'[]'::jsonb
    RETURNING s.id
), affected_routes AS (
    SELECT u.id, text_route.capabilities, EXISTS (
        SELECT 1 FROM jsonb_array_elements(text_route.capabilities) offered
        JOIN jsonb_array_elements(p.definition->'capabilities') requested ON offered->>'operation'=requested->>'operation'
    ) AS matches_public_model
    FROM tokenhub_upstreams u
    JOIN tokenhub_provider_credentials c ON c.id=u.definition->>'credential_id' AND c.subscription_kind<>''
    JOIN tokenhub_public_models p ON p.id=u.public_model_id
    CROSS JOIN LATERAL (
        SELECT jsonb_path_query_array(u.definition, '$.capabilities[*] ? (@.operation == "chat" || @.operation == "responses")') AS capabilities
    ) text_route
    WHERE u.definition @? '$.capabilities[*] ? (@.operation == "image_generation" || @.operation == "image_edit" || @.operation == "video_generation")'
), trimmed_routes AS (
    UPDATE tokenhub_upstreams u
    SET definition=(u.definition - 'video_actions') || jsonb_build_object('capabilities',r.capabilities,'version',u.version+1)
        || CASE WHEN r.matches_public_model THEN '{}'::jsonb
                ELSE '{"enabled":false,"healthy":false,"route_detached":true}'::jsonb END,
        version=u.version+1
    FROM affected_routes r WHERE u.id=r.id
    RETURNING u.public_model_id
), updated_policies AS (
    UPDATE tokenhub_route_policies SET version=version+1
    WHERE model_id IN (SELECT public_model_id FROM trimmed_routes)
)
UPDATE tokenhub_catalog_revisions SET version=version+1 WHERE id='global'
AND (EXISTS (SELECT 1 FROM removed_models) OR EXISTS (SELECT 1 FROM trimmed_models) OR EXISTS (SELECT 1 FROM trimmed_routes));

ALTER TABLE tokenhub_subscription_accounts DROP COLUMN connection_hash;
