package db_test

import (
	"errors"
	"slices"
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

func TestCreateStudent(t *testing.T) {
	u := newTestUsers(t)

	got, err := u.q.CreateStudent(t.Context(), db.CreateStudentParams{
		Login:        "sidorov",
		PasswordHash: "sidorov-hash",
		LastName:     "Сидоров",
		FirstName:    "Иван",
		ClassID:      u.class.ID,
	})
	if err != nil {
		t.Fatalf("CreateStudent: %v", err)
	}
	want := db.CreateStudentRow{ID: got.ID, FirstName: "Иван", LastName: "Сидоров", Login: "sidorov", ClassID: u.class.ID}
	if got.ID <= 0 || got != want {
		t.Errorf("CreateStudent = %+v, want %+v with id > 0", got, want)
	}

	stored, err := u.q.GetUserByLogin(t.Context(), "sidorov")
	if err != nil {
		t.Fatalf("GetUserByLogin(%q): %v", "sidorov", err)
	}
	if stored.ID != got.ID || stored.Role != db.UserRoleStudent || stored.PasswordHash != "sidorov-hash" {
		t.Errorf("stored user = (id %d, %q, hash %q), want (id %d, %q, hash %q)",
			stored.ID, stored.Role, stored.PasswordHash, got.ID, db.UserRoleStudent, "sidorov-hash")
	}
	checkClass(t, stored.ClassID, stored.ClassName, &u.class)
}

func TestCreateStudentConstraints(t *testing.T) {
	tests := []struct {
		name           string
		login          string
		unknownClass   bool
		wantCode       string
		wantConstraint string
	}{
		{"login of another student", "student", false, "23505", "users_login_key"},
		{"login of the teacher", "teacher", false, "23505", "users_login_key"},
		// Приводит логин к нижнему регистру приложение, база только проверяет.
		{"login not in lower case", "Sidorov", false, "23514", "users_login_check"},
		{"unknown class", "sidorov", true, "23503", "users_class_id_fkey"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newTestUsers(t)
			classID := u.class.ID
			if tt.unknownClass {
				classID += 1000
			}

			_, err := u.q.CreateStudent(t.Context(), db.CreateStudentParams{
				Login:        tt.login,
				PasswordHash: "hash",
				LastName:     "Сидоров",
				FirstName:    "Иван",
				ClassID:      classID,
			})
			requirePgError(t, err, tt.wantCode, tt.wantConstraint)
		})
	}
}

func TestListClassStudents(t *testing.T) {
	q := newQueries(t)
	classA := mustCreateClass(t, q, "7А")
	classB := mustCreateClass(t, q, "7Б")
	empty := mustCreateClass(t, q, "8А")

	// Ученики создаются не в том порядке, в каком должны вернуться.
	// Двое Ивановых Антонов — полные тёзки: их порядок задаёт id.
	created := make(map[string]db.CreateStudentRow)
	for _, s := range []struct {
		login, lastName, firstName string
		classID                    int64
	}{
		{"petrov", "Петров", "Пётр", classA.ID},
		{"ivanov.s", "Иванов", "Сергей", classA.ID},
		{"abramov", "Абрамов", "Олег", classB.ID},
		{"ivanov.a1", "Иванов", "Антон", classA.ID},
		{"ivanov.a2", "Иванов", "Антон", classA.ID},
	} {
		row, err := q.CreateStudent(t.Context(), db.CreateStudentParams{
			Login:        s.login,
			PasswordHash: "hash",
			LastName:     s.lastName,
			FirstName:    s.firstName,
			ClassID:      s.classID,
		})
		if err != nil {
			t.Fatalf("CreateStudent(%q): %v", s.login, err)
		}
		created[s.login] = row
	}

	t.Run("sorted by last name, first name and id", func(t *testing.T) {
		got, err := q.ListClassStudents(t.Context(), classA.ID)
		if err != nil {
			t.Fatalf("ListClassStudents: %v", err)
		}

		want := []db.ListClassStudentsRow{
			db.ListClassStudentsRow(created["ivanov.a1"]),
			db.ListClassStudentsRow(created["ivanov.a2"]),
			db.ListClassStudentsRow(created["ivanov.s"]),
			db.ListClassStudentsRow(created["petrov"]),
		}
		if !slices.Equal(got, want) {
			t.Errorf("ListClassStudents(7А) = %+v, want %+v", got, want)
		}
	})

	t.Run("empty class", func(t *testing.T) {
		got, err := q.ListClassStudents(t.Context(), empty.ID)
		if err != nil {
			t.Fatalf("ListClassStudents: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("ListClassStudents(8А) = %+v, want no students", got)
		}
	})
}
