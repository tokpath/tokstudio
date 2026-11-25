CREATE TABLE usage_image_batches (
 id UUID PRIMARY KEY,
 key_id UUID NOT NULL REFERENCES usage_keys(id),
 customer_id UUID NOT NULL,
 brand_id UUID NOT NULL,
 business_key TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 model_id TEXT NOT NULL,
 task_name TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE (key_id,business_key)
);
CREATE INDEX usage_image_batches_key ON usage_image_batches(key_id,created_at DESC,id DESC);

CREATE TABLE usage_image_batch_items (
 batch_id UUID NOT NULL REFERENCES usage_image_batches(id),
 custom_id TEXT NOT NULL,
 position INTEGER NOT NULL,
 request_id UUID NOT NULL UNIQUE REFERENCES usage_requests(id),
 PRIMARY KEY (batch_id,custom_id),
 UNIQUE (batch_id,position)
);
