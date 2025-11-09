-- name: GetUsersByEmail :one
SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO users(email, password_hash) VALUES ($1, $2) RETURNING id, email, created_at, updated_at;
