CREATE INDEX IF NOT EXISTS gateway_request_user_page ON gateway_requests(user_id,started_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS gateway_request_channel_page ON gateway_requests(channel_org_id,started_at DESC,id DESC);
