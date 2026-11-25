ALTER TABLE usage_requests
 ADD COLUMN async_image boolean NOT NULL DEFAULT false,
 ADD COLUMN async_input text NOT NULL DEFAULT '',
 ADD COLUMN delivered_at timestamptz,
 ADD CONSTRAINT usage_async_image_contract CHECK (
   (async_image AND operation IN ('image_generation', 'image_edit') AND NOT streaming AND delivered_at IS NOT NULL)
   OR (NOT async_image AND async_input='')
 );
