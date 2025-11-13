-- name: GetAllUserBatches :many
SELECT b.*, COUNT(i.id) as image_count, COUNT(i.id) FILTER (WHERE i.status = 'pending') AS image_pending_count, COUNT(i.id) FILTER (WHERE i.status = 'processing') AS image_processing_count, COUNT(i.id) FILTER (WHERE i.status = 'completed') AS image_completed_count, COUNT(i.id) FILTER (WHERE i.status = 'failed') AS image_failed_count FROM batches b INNER JOIN images i ON i.batch_id = b.id AND i.deleted_at IS NULL WHERE b.user_id = $1 AND b.deleted_at IS NULL GROUP BY b.id ORDER BY b.created_at DESC;

-- name: GetUserBatchByID :one
SELECT * FROM batches WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: CreateBatch :one
INSERT INTO batches(user_id, name, watermark_key, watermark_url) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: DeleteBatchByID :exec
UPDATE batches SET deleted_at = NOW() WHERE id = $1 AND user_id = $2;

-- name: HardDeleteBatchByID :exec
DELETE FROM batches WHERE id = $1 AND user_id = $2;
