package account

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/password"
	"github.com/MitiaKorotkov/drop-home-works/internal/testdb"
)

func TestNormalizeLogin(t *testing.T) {
	tests := []struct {
		name    string
		login   string
		want    string
		wantErr bool
	}{
		{"mixed case", "Teacher.One", "teacher.one", false},
		{"min length", "aaa", "aaa", false},
		{"max length", strings.Repeat("A", 32), strings.Repeat("a", 32), false},
		{"too short", "ab", "", true},
		{"too long", strings.Repeat("a", 33), "", true},
		{"space inside", "a b", "", true},
		{"cyrillic", "иван", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalizedLogin, err := NormalizeLogin(tt.login)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("NormalizeLogin(%q) error = %v, want ErrInvalidInput", tt.login, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeLogin(%q): %v", tt.login, err)
			}
			if normalizedLogin != tt.want {
				t.Errorf("NormalizeLogin(%q) = %q, want %q", tt.login, normalizedLogin, tt.want)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     string
		wantErr  bool
	}{
		{"correct length", strings.Repeat("я", 30), strings.Repeat("я", 30), false},
		{"too long", strings.Repeat("я", 31), "", true},
		{"trim spaces", "  Иван ", "Иван", false},
		{"empty", "", "", true},
		{"only spaces", "   ", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, err := NormalizeName(tt.username)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("NormalizeName(%q) error = %v, want ErrInvalidInput", tt.username, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeName(%q): %v", tt.username, err)
			}
			if gotName != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.username, gotName, tt.want)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"too short", strings.Repeat("ц", 7), true},
		{"min length", strings.Repeat("ц", 8), false},
		{"max length", strings.Repeat("ц", 128), false},
		{"too long", strings.Repeat("ц", 129), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("ValidatePassword(%q) error = %v, want ErrInvalidInput", tt.password, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePassword(%q): %v", tt.password, err)
			}
		})
	}
}

func newQueries(t *testing.T) *db.Queries {
	t.Helper()
	return db.New(testdb.New(t))
}

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
}

func TestCreateTeacher(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		ctx := context.Background()
		q := newQueries(t)

		id, err := CreateTeacher(ctx, q, TeacherInput{
			Login:     "Teacher.One",
			Password:  "Qwerty123!",
			FirstName: "  Teacher ",
			LastName:  " One   ",
		})
		if err != nil {
			t.Fatalf("CreateTeacher: %v", err)
		}

		teacher, err := q.GetUserByLogin(ctx, "teacher.one")
		if err != nil {
			t.Fatalf("GetUserByLogin(%q): %v", "teacher.one", err)
		}

		if teacher.ID != id {
			t.Errorf("id = %d, want = %d", teacher.ID, id)
		}
		if teacher.Role != db.UserRoleTeacher {
			t.Errorf("role = %q, want = %q", teacher.Role, db.UserRoleTeacher)
		}
		if teacher.Login != "teacher.one" {
			t.Errorf("login = %q, want = %q", teacher.Login, "teacher.one")
		}
		if teacher.FirstName != "Teacher" {
			t.Errorf("first name = %q, want = %q", teacher.FirstName, "Teacher")
		}
		if teacher.LastName != "One" {
			t.Errorf("last name = %q, want = %q", teacher.LastName, "One")
		}
		good, err := password.Verify("Qwerty123!", teacher.PasswordHash)
		if err != nil {
			t.Fatalf("verify stored password hash: %v", err)
		}
		if !good {
			t.Errorf("stored password hash does not match the password passed to CreateTeacher")
		}
	})

	t.Run("second teacher", func(t *testing.T) {
		ctx := context.Background()
		q := newQueries(t)

		_, err := CreateTeacher(ctx, q, TeacherInput{
			Login:     "Teacher.One",
			Password:  "Qwerty123!",
			FirstName: "  Teacher ",
			LastName:  " One   ",
		})
		if err != nil {
			t.Fatalf("CreateTeacher: %v", err)
		}

		_, err = CreateTeacher(ctx, q, TeacherInput{
			Login:     "Teacher.Two",
			Password:  "Abcde321",
			FirstName: "  Student",
			LastName:  "Two   ",
		})
		if !errors.Is(err, ErrTeacherExists) {
			t.Fatalf("error = %v, want error = %v", err, ErrTeacherExists)
		}
	})

	t.Run("existing login", func(t *testing.T) {
		ctx := context.Background()
		pool := testdb.New(t)

		var classID int64
		if err := pool.QueryRow(ctx, `INSERT INTO classes (name) VALUES ('7А') RETURNING id`).Scan(&classID); err != nil {
			t.Fatalf("insert class: %v", err)
		}

		if _, err := pool.Exec(ctx, `INSERT INTO users (role, login, password_hash, last_name, first_name, class_id) VALUES ('student', 'taken', 'x', 'S', 'S', $1)`, classID); err != nil {
			t.Fatalf("insert class: %v", err)
		}

		_, err := CreateTeacher(ctx, db.New(pool), TeacherInput{
			Login:     "Taken",
			Password:  "Abcde321",
			FirstName: "Teacher",
			LastName:  "One",
		})
		if !errors.Is(err, ErrLoginTaken) {
			t.Fatalf("error = %v, want error = %v", err, ErrLoginTaken)
		}
	})
}

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	q := db.New(pool)

	id, err := CreateTeacher(ctx, q, TeacherInput{
		Login:     "Teacher.One",
		Password:  "Qwerty123!",
		FirstName: "Teacher",
		LastName:  "One",
	})
	if err != nil {
		t.Fatalf("create teacher: %v", err)
	}

	tests := []struct {
		name     string
		login    string
		password string
		wantErr  error
	}{
		{"correct password", "teacher.one", "Qwerty123!", nil},
		{"login in other case", "TEACHER.ONE", "Qwerty123!", nil},
		{"wrong password", "teacher.one", "wrong-pass", ErrInvalidCredentials},
		{"unknown login", "nobody", "Qwerty123!", ErrInvalidCredentials},
		{"wrong format", "ab", "Qwerty123!", ErrInvalidCredentials},
		{"empty", "", "", ErrInvalidCredentials},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := Authenticate(ctx, q, tt.login, tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Authenticate(%q, %q) error = %v, want %v", tt.login, tt.password, err, tt.wantErr)
			}
			if err == nil && u.ID != id {
				t.Errorf("Authenticate(%q, %q) returned user id %d, want %d", tt.login, tt.password, u.ID, id)
			}
		})
	}

	t.Run("bad hash", func(t *testing.T) {
		ctx := context.Background()
		pool := testdb.New(t)
		q := db.New(pool)

		_, err := CreateTeacher(ctx, q, TeacherInput{
			Login:     "Teacher.One",
			Password:  "Qwerty123!",
			FirstName: "Teacher",
			LastName:  "One",
		})
		if err != nil {
			t.Fatalf("create teacher: %v", err)
		}

		if _, err := pool.Exec(ctx, "UPDATE users SET password_hash = 'broken'"); err != nil {
			t.Fatalf("corrupt password hash: %v", err)
		}

		_, err = Authenticate(ctx, q, "Teacher.One", "Qwerty123!")
		if !errors.Is(err, password.ErrInvalidHash) || errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Authenticate with a corrupted hash: error = %v, want password.ErrInvalidHash and not ErrInvalidCredentials: a broken hash is a server error (500), not a wrong password (401)", err)
		}
	})
}

func TestUserBySession(t *testing.T) {
	ctx := t.Context()
	pool := testdb.New(t)
	q := db.New(pool)

	teacherID, err := CreateTeacher(ctx, q, TeacherInput{
		Login:     "teacher",
		Password:  "Qwerty123!",
		FirstName: "Мария",
		LastName:  "Иванова",
	})
	if err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}

	class, err := q.CreateClass(ctx, "7А")
	if err != nil {
		t.Fatalf("CreateClass: %v", err)
	}
	var studentID int64
	err = pool.QueryRow(ctx,
		`INSERT INTO users (role, login, password_hash, last_name, first_name, class_id)
		VALUES ('student', 'student', 'x', 'Петров', 'Пётр', $1)
		RETURNING id`,
		class.ID,
	).Scan(&studentID)
	if err != nil {
		t.Fatalf("insert student: %v", err)
	}

	sessions := []db.CreateSessionParams{
		{TokenHash: []byte("teacher"), UserID: teacherID, ExpiresAt: time.Now().Add(time.Hour)},
		{TokenHash: []byte("student"), UserID: studentID, ExpiresAt: time.Now().Add(time.Hour)},
		{TokenHash: []byte("expired"), UserID: teacherID, ExpiresAt: time.Now().Add(-time.Minute)},
	}
	for _, s := range sessions {
		if err := q.CreateSession(ctx, s); err != nil {
			t.Fatalf("CreateSession(%q): %v", s.TokenHash, err)
		}
	}

	t.Run("teacher session", func(t *testing.T) {
		got, err := UserBySession(t.Context(), q, []byte("teacher"))
		if err != nil {
			t.Fatalf("UserBySession: %v", err)
		}
		// У учителя ClassID и ClassName — nil, поэтому структуры можно сравнить через ==.
		want := User{ID: teacherID, Role: db.UserRoleTeacher, FirstName: "Мария", LastName: "Иванова"}
		if got != want {
			t.Errorf("UserBySession = %+v, want %+v", got, want)
		}
	})

	t.Run("student session", func(t *testing.T) {
		got, err := UserBySession(t.Context(), q, []byte("student"))
		if err != nil {
			t.Fatalf("UserBySession: %v", err)
		}
		if got.ID != studentID || got.Role != db.UserRoleStudent {
			t.Errorf("user = (id %d, %q), want (id %d, %q)", got.ID, got.Role, studentID, db.UserRoleStudent)
		}
		if got.ClassID == nil || got.ClassName == nil {
			t.Fatalf("ClassID is nil: %t, ClassName is nil: %t; want class (%d, %q): UserBySession lost the class",
				got.ClassID == nil, got.ClassName == nil, class.ID, class.Name)
		}
		if *got.ClassID != class.ID || *got.ClassName != class.Name {
			t.Errorf("class = (%d, %q), want (%d, %q)", *got.ClassID, *got.ClassName, class.ID, class.Name)
		}
	})

	for _, tt := range []struct{ name, hash string }{
		{"expired session", "expired"},
		{"unknown token", "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u, err := UserBySession(t.Context(), q, []byte(tt.hash))
			if !errors.Is(err, ErrNoSession) {
				t.Errorf("UserBySession(%q) = %+v, %v; want ErrNoSession", tt.hash, u, err)
			}
		})
	}
}

func TestCreateStudent(t *testing.T) {
	// setup — база с учителем (логин "teacher") и классом.
	setup := func(t *testing.T) (*db.Queries, db.Class) {
		t.Helper()
		q := newQueries(t)
		_, err := CreateTeacher(t.Context(), q, TeacherInput{
			Login:     "teacher",
			Password:  "Qwerty123!",
			FirstName: "Мария",
			LastName:  "Иванова",
		})
		if err != nil {
			t.Fatalf("CreateTeacher: %v", err)
		}
		class, err := q.CreateClass(t.Context(), "7А")
		if err != nil {
			t.Fatalf("CreateClass: %v", err)
		}
		return q, class
	}
	valid := func(classID int64) StudentInput {
		return StudentInput{
			Login:     "Petrov.P",
			Password:  "Student123!",
			FirstName: " Пётр ",
			LastName:  "Петров  ",
			ClassID:   classID,
		}
	}

	t.Run("created", func(t *testing.T) {
		q, class := setup(t)

		got, err := CreateStudent(t.Context(), q, valid(class.ID))
		if err != nil {
			t.Fatalf("CreateStudent: %v", err)
		}
		want := db.CreateStudentRow{ID: got.ID, FirstName: "Пётр", LastName: "Петров", Login: "petrov.p", ClassID: class.ID}
		if got.ID <= 0 || got != want {
			t.Errorf("CreateStudent = %+v, want %+v with id > 0: login in lower case, names without spaces around", got, want)
		}

		stored, err := q.GetUserByLogin(t.Context(), "petrov.p")
		if err != nil {
			t.Fatalf("GetUserByLogin(%q): %v", "petrov.p", err)
		}
		if stored.Role != db.UserRoleStudent {
			t.Errorf("role = %q, want %q", stored.Role, db.UserRoleStudent)
		}
		good, err := password.Verify("Student123!", stored.PasswordHash)
		if err != nil {
			t.Fatalf("verify stored password hash: %v", err)
		}
		if !good {
			t.Errorf("stored password hash does not match the password passed to CreateStudent")
		}
	})

	t.Run("login taken", func(t *testing.T) {
		q, class := setup(t)
		if _, err := CreateStudent(t.Context(), q, valid(class.ID)); err != nil {
			t.Fatalf("CreateStudent: %v", err)
		}

		for _, login := range []string{"petrov.p", "PETROV.P", "teacher"} {
			in := valid(class.ID)
			in.Login = login

			_, err := CreateStudent(t.Context(), q, in)
			if !errors.Is(err, ErrLoginTaken) {
				t.Errorf("CreateStudent with login %q: error = %v, want ErrLoginTaken", login, err)
			}
		}
	})

	t.Run("unknown class", func(t *testing.T) {
		q, class := setup(t)

		_, err := CreateStudent(t.Context(), q, valid(class.ID+1000))
		if !errors.Is(err, ErrClassNotFound) {
			t.Fatalf("CreateStudent in class %d: error = %v, want ErrClassNotFound", class.ID+1000, err)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		q, class := setup(t)

		for _, tt := range []struct {
			name   string
			modify func(in *StudentInput)
		}{
			{"bad login", func(in *StudentInput) { in.Login = "ab" }},
			{"empty first name", func(in *StudentInput) { in.FirstName = "" }},
			{"blank last name", func(in *StudentInput) { in.LastName = "   " }},
			{"short password", func(in *StudentInput) { in.Password = "1234567" }},
		} {
			t.Run(tt.name, func(t *testing.T) {
				in := valid(class.ID)
				tt.modify(&in)

				_, err := CreateStudent(t.Context(), q, in)
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("CreateStudent(%+v) error = %v, want ErrInvalidInput", in, err)
				}
			})
		}
	})

	t.Run("database error", func(t *testing.T) {
		pool := testdb.New(t)
		q := db.New(pool)
		pool.Close()

		_, err := CreateStudent(t.Context(), q, valid(1))
		if err == nil || errors.Is(err, ErrLoginTaken) || errors.Is(err, ErrClassNotFound) || errors.Is(err, ErrInvalidInput) {
			t.Fatalf("CreateStudent with the database closed: error = %v, want a server error, not ErrLoginTaken, ErrClassNotFound or ErrInvalidInput", err)
		}
	})
}

func TestUpdateStudent(t *testing.T) {
	// setup — база с учителем, классами 7А и 8А и учеником Петровым Петром в 7А.
	setup := func(t *testing.T) (q *db.Queries, teacherID int64, student db.CreateStudentRow, other db.Class) {
		t.Helper()
		q = newQueries(t)
		teacherID, err := CreateTeacher(t.Context(), q, TeacherInput{
			Login:     "teacher",
			Password:  "Qwerty123!",
			FirstName: "Мария",
			LastName:  "Иванова",
		})
		if err != nil {
			t.Fatalf("CreateTeacher: %v", err)
		}
		class, err := q.CreateClass(t.Context(), "7А")
		if err != nil {
			t.Fatalf("CreateClass(7А): %v", err)
		}
		other, err = q.CreateClass(t.Context(), "8А")
		if err != nil {
			t.Fatalf("CreateClass(8А): %v", err)
		}
		student, err = CreateStudent(t.Context(), q, StudentInput{
			Login:     "petrov",
			Password:  "Student123!",
			FirstName: "Пётр",
			LastName:  "Петров",
			ClassID:   class.ID,
		})
		if err != nil {
			t.Fatalf("CreateStudent: %v", err)
		}
		return q, teacherID, student, other
	}

	t.Run("rename, spaces trimmed", func(t *testing.T) {
		q, _, student, _ := setup(t)

		got, err := UpdateStudent(t.Context(), q, student.ID, StudentUpdate{FirstName: new("  Павел "), LastName: new("Сидоров ")})
		if err != nil {
			t.Fatalf("UpdateStudent: %v", err)
		}
		want := db.UpdateStudentRow{ID: student.ID, FirstName: "Павел", LastName: "Сидоров", Login: "petrov", ClassID: student.ClassID}
		if got != want {
			t.Errorf("UpdateStudent = %+v, want %+v", got, want)
		}
	})

	t.Run("transfer, names kept", func(t *testing.T) {
		q, _, student, other := setup(t)

		got, err := UpdateStudent(t.Context(), q, student.ID, StudentUpdate{ClassID: &other.ID})
		if err != nil {
			t.Fatalf("UpdateStudent: %v", err)
		}
		want := db.UpdateStudentRow{ID: student.ID, FirstName: "Пётр", LastName: "Петров", Login: "petrov", ClassID: other.ID}
		if got != want {
			t.Errorf("UpdateStudent = %+v, want %+v", got, want)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		// Ни один случай ничего не меняет, поэтому база у всех общая.
		q, teacherID, student, _ := setup(t)

		for _, tt := range []struct {
			name    string
			id      int64
			in      StudentUpdate
			wantErr error
		}{
			{"nothing to update", student.ID, StudentUpdate{}, ErrInvalidInput},
			{"blank first name", student.ID, StudentUpdate{FirstName: new("   ")}, ErrInvalidInput},
			{"too long last name", student.ID, StudentUpdate{LastName: new(strings.Repeat("я", 31))}, ErrInvalidInput},
			{"unknown class", student.ID, StudentUpdate{ClassID: new(student.ClassID + 1000)}, ErrClassNotFound},
			{"unknown student", student.ID + 1000, StudentUpdate{FirstName: new("Павел")}, ErrStudentNotFound},
			{"teacher", teacherID, StudentUpdate{FirstName: new("Павел")}, ErrStudentNotFound},
		} {
			t.Run(tt.name, func(t *testing.T) {
				row, err := UpdateStudent(t.Context(), q, tt.id, tt.in)
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("UpdateStudent(id %d) = %+v, %v; want %v", tt.id, row, err, tt.wantErr)
				}
			})
		}

		stored, err := q.GetUserByLogin(t.Context(), "petrov")
		if err != nil {
			t.Fatalf("GetUserByLogin(%q): %v", "petrov", err)
		}
		if stored.FirstName != "Пётр" || stored.LastName != "Петров" {
			t.Errorf("student's name after rejected updates = %q %q, want %q %q", stored.FirstName, stored.LastName, "Пётр", "Петров")
		}
		if stored.ClassID == nil {
			t.Fatalf("student's class after rejected updates is nil, want %d", student.ClassID)
		}
		if *stored.ClassID != student.ClassID {
			t.Errorf("student's class after rejected updates = %d, want %d", *stored.ClassID, student.ClassID)
		}
	})

	t.Run("database error", func(t *testing.T) {
		pool := testdb.New(t)
		q := db.New(pool)
		pool.Close()

		_, err := UpdateStudent(t.Context(), q, 1, StudentUpdate{FirstName: new("Павел")})
		if err == nil || errors.Is(err, ErrStudentNotFound) || errors.Is(err, ErrClassNotFound) || errors.Is(err, ErrInvalidInput) {
			t.Fatalf("UpdateStudent with the database closed: error = %v, want a server error, not ErrStudentNotFound, ErrClassNotFound or ErrInvalidInput", err)
		}
	})
}

func TestSetStudentPassword(t *testing.T) {
	const oldPassword, newPassword = "Student123!", "NewPass456?"

	// setup — база с учителем, классом и учеником petrov со старым паролем.
	setup := func(t *testing.T) (q *db.Queries, teacherID, studentID int64) {
		t.Helper()
		q = newQueries(t)
		teacherID, err := CreateTeacher(t.Context(), q, TeacherInput{
			Login:     "teacher",
			Password:  "Qwerty123!",
			FirstName: "Мария",
			LastName:  "Иванова",
		})
		if err != nil {
			t.Fatalf("CreateTeacher: %v", err)
		}
		class, err := q.CreateClass(t.Context(), "7А")
		if err != nil {
			t.Fatalf("CreateClass: %v", err)
		}
		student, err := CreateStudent(t.Context(), q, StudentInput{
			Login:     "petrov",
			Password:  oldPassword,
			FirstName: "Пётр",
			LastName:  "Петров",
			ClassID:   class.ID,
		})
		if err != nil {
			t.Fatalf("CreateStudent: %v", err)
		}
		return q, teacherID, student.ID
	}
	requireAuth := func(t *testing.T, q *db.Queries, login, passwd string, wantErr error) {
		t.Helper()
		if _, err := Authenticate(t.Context(), q, login, passwd); !errors.Is(err, wantErr) {
			t.Errorf("Authenticate(%q, %q) error = %v, want %v", login, passwd, err, wantErr)
		}
	}

	t.Run("changed", func(t *testing.T) {
		q, _, studentID := setup(t)

		if err := SetStudentPassword(t.Context(), q, studentID, newPassword); err != nil {
			t.Fatalf("SetStudentPassword: %v", err)
		}

		requireAuth(t, q, "petrov", newPassword, nil)
		requireAuth(t, q, "petrov", oldPassword, ErrInvalidCredentials)
	})

	t.Run("rejected", func(t *testing.T) {
		// Ни один случай ничего не меняет, поэтому база у всех общая.
		q, teacherID, studentID := setup(t)

		for _, tt := range []struct {
			name    string
			id      int64
			passwd  string
			wantErr error
		}{
			{"too short", studentID, "1234567", ErrInvalidInput},
			{"too long", studentID, strings.Repeat("я", 129), ErrInvalidInput},
			{"unknown student", studentID + 1000, newPassword, ErrStudentNotFound},
			{"teacher", teacherID, newPassword, ErrStudentNotFound},
		} {
			t.Run(tt.name, func(t *testing.T) {
				if err := SetStudentPassword(t.Context(), q, tt.id, tt.passwd); !errors.Is(err, tt.wantErr) {
					t.Errorf("SetStudentPassword(id %d) error = %v, want %v", tt.id, err, tt.wantErr)
				}
			})
		}

		requireAuth(t, q, "petrov", oldPassword, nil)
		requireAuth(t, q, "teacher", "Qwerty123!", nil)
	})

	t.Run("database error", func(t *testing.T) {
		pool := testdb.New(t)
		q := db.New(pool)
		pool.Close()

		err := SetStudentPassword(t.Context(), q, 1, newPassword)
		if err == nil || errors.Is(err, ErrStudentNotFound) || errors.Is(err, ErrInvalidInput) {
			t.Fatalf("SetStudentPassword with the database closed: error = %v, want a server error, not ErrStudentNotFound or ErrInvalidInput", err)
		}
	})
}
