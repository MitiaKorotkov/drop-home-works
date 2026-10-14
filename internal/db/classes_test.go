package db_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/testdb"
	"github.com/jackc/pgx/v5/pgconn"
)

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

func TestCreateClassDifferentCase(t *testing.T) {
	q := newQueries(t)

	mustCreateClass(t, q, "7Б")
	_, err := q.CreateClass(t.Context(), "7б")
	if err != nil {
		t.Fatalf("CreateClass(%q) after %q: %v, want no error: names that differ only in case are different classes", "7б", "7Б", err)
	}
}

func TestCreateClassConstraints(t *testing.T) {
	tests := []struct {
		name           string
		existing       []string
		className      string
		wantCode       string
		wantConstraint string
	}{
		{"duplicate name", []string{"7А"}, "7А", "23505", "classes_name_key"},
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
