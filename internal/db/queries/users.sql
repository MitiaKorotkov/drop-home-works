-- name: GetUserByLogin :one
SELECT u.id, u.role, u.login, u.password_hash, u.first_name, u.last_name, u.class_id, c.name AS class_name
FROM users u
LEFT JOIN classes c ON c.id = u.class_id
WHERE u.login = $1;

-- name: CreateTeacher :one
INSERT INTO users (role, login, password_hash, last_name, first_name)
VALUES ('teacher', $1, $2, $3, $4)
RETURNING id;

-- name: TeacherExists :one
SELECT EXISTS (SELECT 1 FROM users WHERE role = 'teacher');
