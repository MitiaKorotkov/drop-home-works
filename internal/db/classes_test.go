package db_test

import (
	"slices"
	"testing"
	"time"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
)

func TestCreateClass(t *testing.T) {
	q := newQueries(t)

	c := mustCreateClass(t, q, "7А")
	if c.ID <= 0 {
		t.Fatalf("ID = %d, want > 0", c.ID)
	}
	if c.Name != "7А" {
		t.Fatalf("Name = %q, want %q", c.Name, "7А")
	}
	if c.CreatedAt.IsZero() || time.Since(c.CreatedAt).Abs() >= time.Minute {
		t.Fatalf("CreatedAt = %v, want a non-zero time within a minute of now (now = %v)", c.CreatedAt, time.Now())
	}
}

func TestListClasses(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		q := newQueries(t)

		classes, err := q.ListClasses(t.Context())
		if err != nil {
			t.Fatalf("ListClasses on empty table: %v", err)
		}
		if len(classes) != 0 {
			t.Fatalf("ListClasses on empty table = %v, want no classes", classes)
		}
	})

	t.Run("sorted by name", func(t *testing.T) {
		q := newQueries(t)

		class7B := mustCreateClass(t, q, "7Б")
		class11A := mustCreateClass(t, q, "11А")
		class7A := mustCreateClass(t, q, "7А")

		classes, err := q.ListClasses(t.Context())
		if err != nil {
			t.Fatalf("ListClasses: %v", err)
		}

		want := []db.ListClassesRow{
			{ID: class11A.ID, Name: "11А"},
			{ID: class7A.ID, Name: "7А"},
			{ID: class7B.ID, Name: "7Б"},
		}

		if !slices.Equal(classes, want) {
			t.Fatalf("ListClasses = %+v, want %+v", classes, want)
		}
	})
}

func TestCreateClassConstraints(t *testing.T) {
	tests := []struct {
		name           string
		existing       []string
		className      string
		wantCode       string
		wantConstraint string
	}{
		{"duplicate name", []string{"7А"}, "7А", "23505", "classes_name_lower_key"},
		{"same name in another case", []string{"7Б"}, "7б", "23505", "classes_name_lower_key"},
		{"empty name", nil, "", "23514", "classes_name_check"},
		{"space only", nil, " ", "23514", "classes_name_check"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newQueries(t)
			for _, name := range tt.existing {
				mustCreateClass(t, q, name)
			}

			_, err := q.CreateClass(t.Context(), tt.className)
			requirePgError(t, err, tt.wantCode, tt.wantConstraint)
		})
	}
}
