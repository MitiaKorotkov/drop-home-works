package account

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/password"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrTeacherExists = errors.New("teacher already exists")
	ErrLoginTaken    = errors.New("login already taken")
)

type TeacherInput struct {
	Login     string
	Password  string
	FirstName string
	LastName  string
}

var loginRe = regexp.MustCompile("^[a-z0-9._-]{3,32}$")

func NormalizeLogin(login string) (string, error) {
	login = strings.ToLower(login)

	if !loginRe.MatchString(login) {
		return "", fmt.Errorf("%w: must be 3–32 characters: latin letters, digits, '.', '_' or '-'", ErrInvalidInput)
	}

	return login, nil
}

func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || 30 < n {
		return "", fmt.Errorf("%w: must be 1-30 characters, got %d", ErrInvalidInput, n)
	}

	return name, nil
}

func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < 8 || 128 < n {
		return fmt.Errorf("%w: must be 8–128 characters", ErrInvalidInput)
	}

	return nil
}

func CreateTeacher(ctx context.Context, q *db.Queries, in TeacherInput) (int64, error) {
	login, err := NormalizeLogin(in.Login)
	if err != nil {
		return 0, fmt.Errorf("login: %w", err)
	}
	firstName, err := NormalizeName(in.FirstName)
	if err != nil {
		return 0, fmt.Errorf("first name: %w", err)
	}
	lastName, err := NormalizeName(in.LastName)
	if err != nil {
		return 0, fmt.Errorf("last name: %w", err)
	}
	if err := ValidatePassword(in.Password); err != nil {
		return 0, fmt.Errorf("password: %w", err)
	}

	exists, err := q.TeacherExists(ctx)
	if err != nil {
		return 0, fmt.Errorf("check existing teacher: %w", err)
	}
	if exists {
		return 0, ErrTeacherExists
	}

	id, err := q.CreateTeacher(ctx, db.CreateTeacherParams{
		Login:        login,
		PasswordHash: password.Hash(in.Password),
		LastName:     lastName,
		FirstName:    firstName,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_login_key" {
		return 0, ErrLoginTaken
	}
	if err != nil {
		return 0, fmt.Errorf("insert teacher: %w", err)
	}

	return id, nil
}
