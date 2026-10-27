package classes

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/testdb"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name      string
		className string
		want      string
		wantErr   bool
	}{
		{"plain", "7А", "7А", false},
		{"trim spaces", "  7А ", "7А", false},
		{"inner space is kept", "Кружок 7", "Кружок 7", false},
		// 30 кириллических букв — это 60 байт: длина считается в символах.
		{"max length", strings.Repeat("я", 30), strings.Repeat("я", 30), false},
		{"too long", strings.Repeat("я", 31), "", true},
		{"empty", "", "", true},
		{"only spaces", "   ", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeName(tt.className)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("NormalizeName(%q) error = %v, want ErrInvalidInput", tt.className, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeName(%q): %v", tt.className, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.className, got, tt.want)
			}
		})
	}
}

func TestCreateClass(t *testing.T) {
	t.Run("created", func(t *testing.T) {
		q := db.New(testdb.New(t))

		class, err := CreateClass(t.Context(), q, "  7А ")
		if err != nil {
			t.Fatalf("CreateClass(%q): %v", "  7А ", err)
		}
		if class.ID <= 0 || class.Name != "7А" {
			t.Errorf("CreateClass(%q) = (id %d, %q), want (id > 0, %q)", "  7А ", class.ID, class.Name, "7А")
		}

		rows, err := q.ListClasses(t.Context())
		if err != nil {
			t.Fatalf("ListClasses: %v", err)
		}
		want := []db.ListClassesRow{{ID: class.ID, Name: "7А"}}
		if !slices.Equal(rows, want) {
			t.Errorf("classes in the database = %+v, want %+v", rows, want)
		}
	})

	for _, tt := range []struct{ name, existing, className string }{
		{"same name", "7А", "7А"},
		// Повтор находится только после нормализации: в базе лежит «7А» без пробелов.
		{"same name with spaces around", "7А", " 7А "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := db.New(testdb.New(t))
			if _, err := CreateClass(t.Context(), q, tt.existing); err != nil {
				t.Fatalf("CreateClass(%q): %v", tt.existing, err)
			}

			_, err := CreateClass(t.Context(), q, tt.className)
			if !errors.Is(err, ErrNameTaken) {
				t.Fatalf("CreateClass(%q) after %q: error = %v, want ErrNameTaken", tt.className, tt.existing, err)
			}
		})
	}

	t.Run("invalid name", func(t *testing.T) {
		q := db.New(testdb.New(t))

		// Без проверки в Go название из пробелов дошло бы до CHECK в базе
		// и вернулось бы ошибкой PostgreSQL, то есть ответом 500 вместо 400.
		_, err := CreateClass(t.Context(), q, "   ")
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("CreateClass(%q) error = %v, want ErrInvalidInput", "   ", err)
		}
	})

	t.Run("database error", func(t *testing.T) {
		pool := testdb.New(t)
		q := db.New(pool)
		pool.Close()

		_, err := CreateClass(t.Context(), q, "7А")
		if err == nil || errors.Is(err, ErrNameTaken) || errors.Is(err, ErrInvalidInput) {
			t.Fatalf("CreateClass with the database closed: error = %v, want a non-nil error that is neither ErrNameTaken nor ErrInvalidInput: a database failure is a server error (500), not a client error", err)
		}
	})
}
