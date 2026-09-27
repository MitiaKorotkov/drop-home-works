-- +goose Up

CREATE TABLE classes (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       text NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TYPE user_role AS ENUM ('teacher', 'student');

CREATE TABLE users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    role          user_role NOT NULL,
    -- Приложение приводит введённый логин к нижнему регистру.
    login         text NOT NULL UNIQUE CHECK (login ~ '^[a-z0-9._-]{3,32}$'),
    password_hash text NOT NULL,
    last_name     text NOT NULL CHECK (btrim(last_name) <> ''),
    first_name    text NOT NULL CHECK (btrim(first_name) <> ''),
    -- Ученик состоит ровно в одном классе, у учителя класса нет.
    class_id      bigint REFERENCES classes (id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK ((role = 'student') = (class_id IS NOT NULL))
);

CREATE INDEX users_class_id_idx ON users (class_id);

CREATE TABLE sessions (
    -- SHA-256 от токена из cookie. Сам токен в базе не хранится, поэтому
    -- утечка базы или бэкапа не даёт войти под чужой сессией.
    token_hash bytea PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- Реестр всех файлов в хранилище. Имя файла на диске — его id.
CREATE TABLE files (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    original_name text NOT NULL CHECK (original_name <> '' AND length(original_name) <= 255),
    content_type  text NOT NULL,
    size_bytes    bigint NOT NULL CHECK (size_bytes >= 0),
    created_at    timestamptz NOT NULL DEFAULT now(),
    -- С какого момента на файл никто не ссылается; NULL — файл используется.
    -- Новый файл считается сиротой, пока его не привяжут к сдаче или заданию.
    -- Фоновая очистка удаляет файлы, которые остаются сиротами дольше суток.
    orphaned_at   timestamptz DEFAULT now()
);

CREATE INDEX files_orphaned_at_idx ON files (orphaned_at) WHERE orphaned_at IS NOT NULL;

CREATE TABLE assignments (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title       text NOT NULL CHECK (btrim(title) <> ''),
    description text NOT NULL DEFAULT '',
    task_count  integer NOT NULL CHECK (task_count > 0),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE assignment_files (
    file_id       uuid PRIMARY KEY REFERENCES files (id),
    assignment_id bigint NOT NULL REFERENCES assignments (id) ON DELETE CASCADE
);

CREATE INDEX assignment_files_assignment_id_idx ON assignment_files (assignment_id);

-- Задание, выданное классу, с дедлайном для этого класса.
CREATE TABLE class_assignments (
    class_id      bigint NOT NULL REFERENCES classes (id),
    assignment_id bigint NOT NULL REFERENCES assignments (id) ON DELETE CASCADE,
    deadline      timestamptz NOT NULL,
    PRIMARY KEY (class_id, assignment_id)
);

CREATE INDEX class_assignments_assignment_id_idx ON class_assignments (assignment_id);

-- Состояния «Не сдано» и «Просрочено» не хранятся: это отсутствие строки
-- в submissions, а какое из двух — определяет сравнение с дедлайном.
CREATE TYPE submission_status AS ENUM ('submitted', 'in_review', 'reviewed');

CREATE TABLE submissions (
    student_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    assignment_id bigint NOT NULL REFERENCES assignments (id) ON DELETE CASCADE,
    file_id       uuid NOT NULL UNIQUE REFERENCES files (id),
    status        submission_status NOT NULL DEFAULT 'submitted',
    -- Не больше assignments.task_count: проверяется в запросах приложения.
    score         integer CHECK (score >= 0),
    submitted_at  timestamptz NOT NULL,
    PRIMARY KEY (student_id, assignment_id),
    CHECK ((status = 'reviewed') = (score IS NOT NULL))
);

CREATE INDEX submissions_assignment_id_idx ON submissions (assignment_id);

-- Поддерживает files.orphaned_at: файл, на который перестали ссылаться,
-- помечается сиротой, а с привязанного файла пометка снимается.
-- Срабатывает и при каскадном удалении задания или ученика.
-- +goose StatementBegin
CREATE FUNCTION track_file_refs() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        UPDATE files SET orphaned_at = now() WHERE id = OLD.file_id;
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        UPDATE files SET orphaned_at = NULL WHERE id = NEW.file_id;
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER submissions_track_file_refs
    AFTER INSERT OR UPDATE OF file_id OR DELETE ON submissions
    FOR EACH ROW EXECUTE FUNCTION track_file_refs();

CREATE TRIGGER assignment_files_track_file_refs
    AFTER INSERT OR UPDATE OF file_id OR DELETE ON assignment_files
    FOR EACH ROW EXECUTE FUNCTION track_file_refs();

-- +goose Down

DROP TABLE submissions;
DROP TYPE submission_status;
DROP TABLE class_assignments;
DROP TABLE assignment_files;
DROP TABLE assignments;
DROP FUNCTION track_file_refs();
DROP TABLE files;
DROP TABLE sessions;
DROP TABLE users;
DROP TYPE user_role;
DROP TABLE classes;
