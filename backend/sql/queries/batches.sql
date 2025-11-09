-- name: CreateBatch :one
INSERT INTO batches(user_id, name, watermark_text, watermark_url) VALUES ($1, $2, $3, $4) RETURNING *;
