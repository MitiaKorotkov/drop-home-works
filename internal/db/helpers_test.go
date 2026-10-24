package db_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/testdb"
	"github.com/jackc/pgx/v5/pgconn"
)

type testUsers struct {
	q         *db.Queries
	class     db.Class
	teacherID int64
	studentID int64
}

func newTestUsers(t *testing.T) testUsers {
	t.Helper()

	pool := testdb.New(t)
	q := db.New(pool)

	class := mustCreateClass(t, q, "7А")

	teacherID, err := q.CreateTeacher(t.Context(), db.CreateTeacherParams{
		Login:        "teacher",
		PasswordHash: "teacher-hash",
		LastName:     "Иванова",
		FirstName:    "Мария",
	})
	if err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}

	var studentID int64
	err = pool.QueryRow(t.Context(),
		`INSERT INTO users (role, login, password_hash, last_name, first_name, class_id)
		VALUES ('student', 'student', 'student-hash', 'Петров', 'Пётр', $1)
		RETURNING id`,
		class.ID,
	).Scan(&studentID)
	if err != nil {
		t.Fatalf("insert student: %v", err)
	}

	return testUsers{q: q, class: class, teacherID: teacherID, studentID: studentID}
}

func show[T any](p *T) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%#v", *p)
}

func checkClass(t *testing.T, gotID *int64, gotName *string, want *db.Class) {
	t.Helper()

	if want == nil {
		if gotID != nil || gotName != nil {
			t.Errorf("class = (%s, %s), want (nil, nil): a teacher has no class", show(gotID), show(gotName))
		}
		return
	}
	if gotID == nil || *gotID != want.ID || gotName == nil || *gotName != want.Name {
		t.Errorf("class = (%s, %s), want (%d, %q)", show(gotID), show(gotName), want.ID, want.Name)
	}
}

func newQueries(t *testing.T) *db.Queries {
	t.Helper()
	return db.New(testdb.New(t))
}

func requirePgError(t *testing.T, err error, wantCode, wantConstraint string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want PostgreSQL error %s (%s)", wantCode, wantConstraint)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error = %v, want PostgreSQL error %s (%s)", err, wantCode, wantConstraint)
	}
	if pgErr.Code != wantCode || pgErr.ConstraintName != wantConstraint {
		t.Fatalf("PostgreSQL error %s (%q), want %s (%q)", pgErr.Code, pgErr.ConstraintName, wantCode, wantConstraint)
	}
}

func mustCreateClass(t *testing.T, q *db.Queries, name string) db.Class {
	t.Helper()

	c, err := q.CreateClass(context.Background(), name)
	if err != nil {
		t.Fatalf("CreateClass(%q): %v", name, err)
	}

	return c
}
