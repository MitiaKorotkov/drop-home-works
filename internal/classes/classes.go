package classes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNameTaken    = errors.New("name already taken")
)

func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || 30 < n {
		return "", fmt.Errorf("%w: must be 1-30 characters, got %d", ErrInvalidInput, n)
	}

	return name, nil
}

func CreateClass(ctx context.Context, q *db.Queries, name string) (db.Class, error) {
	name, err := NormalizeName(name)
	if err != nil {
		return db.Class{}, fmt.Errorf("name: %w", err)
	}

	class, err := q.CreateClass(ctx, name)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "classes_name_key" {
		return db.Class{}, ErrNameTaken
	}
	if err != nil {
		return db.Class{}, fmt.Errorf("create class: %w", err)
	}

	return class, nil
}
