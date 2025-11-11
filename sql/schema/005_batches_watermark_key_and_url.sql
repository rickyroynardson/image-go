-- +goose up
ALTER TABLE batches DROP COLUMN watermark_text;
ALTER TABLE batches ADD COLUMN watermark_key VARCHAR(255);

-- +goose down
ALTER TABLE batches DROP COLUMN watermark_key;
ALTER TABLE batches ADD COLUMN watermark_text TEXT;
