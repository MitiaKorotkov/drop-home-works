-- name: CreateClass :one
INSERT INTO classes (name) VALUES ($1)
RETURNING id, name, created_at;

-- name: ListClasses :many
SELECT id, name
FROM classes
ORDER BY name;