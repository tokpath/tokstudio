CREATE INDEX usage_requests_completed_period
    ON usage_requests(operator_id, completed_at) WHERE status='completed';
