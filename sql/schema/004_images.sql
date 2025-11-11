-- +goose up
CREATE TYPE image_status AS ENUM ('pending', 'processing', 'completed', 'failed');
CREATE TABLE images(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id UUID NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    original_url TEXT NOT NULL,
    processed_url TEXT,
    status image_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- +goose down
DROP TYPE IF EXISTS image_status CASCADE;
DROP TABLE images;
