-- name: CreateImage :one
INSERT INTO images(batch_id, original_url) VALUES($1, $2) RETURNING *;
