package account

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

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
