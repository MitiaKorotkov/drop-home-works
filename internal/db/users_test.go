package db_test

import (
	"errors"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/jackc/pgx/v5"
)

func TestGetUserByLogin(t *testing.T) {
	t.Run("teacher", func(t *testing.T) {
		u := newTestUsers(t)

		got, err := u.q.GetUserByLogin(t.Context(), "teacher")
		if err != nil {
			t.Fatalf("GetUserByLogin(%q): %v", "teacher", err)
		}
		if got.ID != u.teacherID || got.Role != db.UserRoleTeacher || got.Login != "teacher" {
			t.Errorf("user = (id %d, %q, %q), want (id %d, %q, %q)", got.ID, got.Role, got.Login, u.teacherID, db.UserRoleTeacher, "teacher")
		}
		if got.PasswordHash != "teacher-hash" {
			t.Errorf("password hash = %q, want %q", got.PasswordHash, "teacher-hash")
		}
		if got.FirstName != "Мария" || got.LastName != "Иванова" {
			t.Errorf("name = %q %q, want %q %q", got.FirstName, got.LastName, "Мария", "Иванова")
		}
		checkClass(t, got.ClassID, got.ClassName, nil)
	})

	t.Run("student", func(t *testing.T) {
		u := newTestUsers(t)

		got, err := u.q.GetUserByLogin(t.Context(), "student")
		if err != nil {
			t.Fatalf("GetUserByLogin(%q): %v", "student", err)
		}
		if got.ID != u.studentID || got.Role != db.UserRoleStudent || got.Login != "student" {
			t.Errorf("user = (id %d, %q, %q), want (id %d, %q, %q)", got.ID, got.Role, got.Login, u.studentID, db.UserRoleStudent, "student")
		}
		if got.PasswordHash != "student-hash" {
			t.Errorf("password hash = %q, want %q", got.PasswordHash, "student-hash")
		}
		checkClass(t, got.ClassID, got.ClassName, &u.class)
	})

	t.Run("unknown login", func(t *testing.T) {
		u := newTestUsers(t)

		row, err := u.q.GetUserByLogin(t.Context(), "nobody")
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("GetUserByLogin(%q) = %+v, %v; want pgx.ErrNoRows", "nobody", row, err)
		}
	})
}
