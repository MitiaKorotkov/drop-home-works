package db_test

import (
	"errors"
	"testing"
	"time"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/jackc/pgx/v5"
)

func mustCreateSession(t *testing.T, q *db.Queries, hash []byte, userID int64, expiresAt time.Time) {
	t.Helper()

	err := q.CreateSession(t.Context(), db.CreateSessionParams{
		TokenHash: hash,
		UserID:    userID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateSession(%q, user %d): %v", hash, userID, err)
	}
}

func requireNoSession(t *testing.T, q *db.Queries, hash []byte) {
	t.Helper()

	row, err := q.GetSessionUser(t.Context(), hash)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("GetSessionUser(%q) = %+v, %v; want pgx.ErrNoRows", hash, row, err)
	}
}

func TestGetSessionUser(t *testing.T) {
	inHour := time.Now().Add(time.Hour)

	t.Run("teacher", func(t *testing.T) {
		u := newTestUsers(t)
		hash := []byte("teacher-session")
		mustCreateSession(t, u.q, hash, u.teacherID, inHour)

		got, err := u.q.GetSessionUser(t.Context(), hash)
		if err != nil {
			t.Fatalf("GetSessionUser: %v", err)
		}
		if got.ID != u.teacherID || got.Role != db.UserRoleTeacher {
			t.Errorf("user = (id %d, %q), want (id %d, %q)", got.ID, got.Role, u.teacherID, db.UserRoleTeacher)
		}
		if got.FirstName != "Мария" || got.LastName != "Иванова" {
			t.Errorf("name = %q %q, want %q %q", got.FirstName, got.LastName, "Мария", "Иванова")
		}
		checkClass(t, got.ClassID, got.ClassName, nil)
	})

	t.Run("student", func(t *testing.T) {
		u := newTestUsers(t)
		hash := []byte("student-session")
		mustCreateSession(t, u.q, hash, u.studentID, inHour)

		got, err := u.q.GetSessionUser(t.Context(), hash)
		if err != nil {
			t.Fatalf("GetSessionUser: %v", err)
		}
		if got.ID != u.studentID || got.Role != db.UserRoleStudent {
			t.Errorf("user = (id %d, %q), want (id %d, %q)", got.ID, got.Role, u.studentID, db.UserRoleStudent)
		}
		if got.FirstName != "Пётр" || got.LastName != "Петров" {
			t.Errorf("name = %q %q, want %q %q", got.FirstName, got.LastName, "Пётр", "Петров")
		}
		checkClass(t, got.ClassID, got.ClassName, &u.class)
	})

	t.Run("expired", func(t *testing.T) {
		u := newTestUsers(t)
		hash := []byte("expired-session")
		mustCreateSession(t, u.q, hash, u.teacherID, time.Now().Add(-time.Minute))

		requireNoSession(t, u.q, hash)
	})

	t.Run("unknown token", func(t *testing.T) {
		u := newTestUsers(t)
		// Чужая сессия в таблице есть: запрос должен искать по хешу, а не вернуть первую попавшуюся.
		mustCreateSession(t, u.q, []byte("other-session"), u.teacherID, inHour)

		requireNoSession(t, u.q, []byte("unknown-session"))
	})
}

func TestDeleteSession(t *testing.T) {
	inHour := time.Now().Add(time.Hour)

	t.Run("deletes only this session", func(t *testing.T) {
		u := newTestUsers(t)
		phone, laptop := []byte("phone-session"), []byte("laptop-session")
		mustCreateSession(t, u.q, phone, u.teacherID, inHour)
		mustCreateSession(t, u.q, laptop, u.teacherID, inHour)

		if err := u.q.DeleteSession(t.Context(), phone); err != nil {
			t.Fatalf("DeleteSession(%q): %v", phone, err)
		}

		requireNoSession(t, u.q, phone)
		if _, err := u.q.GetSessionUser(t.Context(), laptop); err != nil {
			t.Errorf("GetSessionUser(%q) after deleting %q: %v, want the session to survive: logout must end only the current session", laptop, phone, err)
		}
	})

	t.Run("unknown token", func(t *testing.T) {
		q := newQueries(t)

		if err := q.DeleteSession(t.Context(), []byte("unknown-session")); err != nil {
			t.Errorf("DeleteSession(unknown token): %v, want nil: a repeated logout must not fail", err)
		}
	})
}
