-- Let the ordinary deadline worker settle outstanding reservations from the retired routes.
UPDATE usage_requests r SET usage_deadline=statement_timestamp()
WHERE r.operation IN ('image_generation','image_edit','video_generation')
  AND r.usage_deadline>statement_timestamp()
  AND (
    (r.status IN ('accepted','executing') AND jsonb_array_length(r.candidates)>0 AND NOT EXISTS (
        SELECT 1 FROM jsonb_array_elements(r.candidates) candidate
        WHERE COALESCE(candidate->>'subscription_kind','')=''
    ))
    OR (r.status IN ('submitted','awaiting_usage') AND (
        SELECT COALESCE(a.definition->>'subscription_kind','') FROM usage_attempts a
        WHERE a.request_id=r.id ORDER BY a.started_at DESC,a.id DESC LIMIT 1
    )<>'')
  );
