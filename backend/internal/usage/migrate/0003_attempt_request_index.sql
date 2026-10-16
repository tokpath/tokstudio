-- Provider task lookups, scoped cost exports and customer request evidence all
-- read attempts by the customer request. Foreign keys do not create an index.
CREATE INDEX usage_attempts_request ON usage_attempts(request_id, started_at DESC, id);
