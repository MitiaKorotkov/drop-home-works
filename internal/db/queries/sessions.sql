-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: GetSessionUser :one
SELECT u.id, u.role, u.first_name, u.last_name, u.class_id, c.name AS class_name
FROM sessions s
JOIN users u ON u.id = s.user_id
LEFT JOIN classes c ON c.id = u.class_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = $1;
