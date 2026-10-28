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

-- name: CreateStudent :one
INSERT INTO users (role, login, password_hash, last_name, first_name, class_id)
VALUES ('student', $1, $2, $3, $4, sqlc.arg(class_id)::bigint)
RETURNING id, first_name, last_name, login, class_id::bigint AS class_id;

-- name: ListClassStudents :many
SELECT id, first_name, last_name, login, class_id::bigint AS class_id
FROM users
WHERE class_id = sqlc.arg(class_id)::bigint
ORDER BY last_name, first_name, id;

-- name: UpdateStudent :one
UPDATE users
SET first_name = coalesce(sqlc.narg(first_name), first_name),
    last_name = coalesce(sqlc.narg(last_name), last_name),
    class_id = coalesce(sqlc.narg(class_id), class_id)
WHERE id = sqlc.arg(id) AND role = 'student'
RETURNING id, first_name, last_name, login, class_id::bigint AS class_id;

-- name: DeleteStudent :execrows
DELETE FROM users
WHERE id = $1 AND role = 'student';
