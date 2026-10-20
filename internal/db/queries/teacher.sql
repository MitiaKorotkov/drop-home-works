-- name: CreateTeacher :one
INSERT INTO users (role, login, password_hash, last_name, first_name)
VALUES ('teacher', $1, $2, $3, $4)
RETURNING id;

-- name: TeacherExists :one
SELECT EXISTS (SELECT 1 FROM users WHERE role = 'teacher');