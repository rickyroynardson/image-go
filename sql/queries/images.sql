-- name: CreateImage :one
INSERT INTO images(batch_id, key, original_url) VALUES($1, $2, $3) RETURNING *;

-- name: GetImageByID :one
SELECT i.*, b.watermark_url, b.watermark_text FROM images i INNER JOIN batches b ON b.id = i.batch_id WHERE i.id = $1 AND i.deleted_at IS NULL AND b.deleted_at IS NULL;

-- name: UpdateImageByID :exec
UPDATE images SET processed_url = $1, status = $2, updated_at = NOW() WHERE id = $3 AND deleted_at IS NULL;
