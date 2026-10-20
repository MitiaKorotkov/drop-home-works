-- name: GetUserByLogin :one
SELECT id, role, login, password_hash, first_name, last_name, class_id
FROM users
WHERE login = $1;