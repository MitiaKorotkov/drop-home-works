package testdb

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	adminURL = os.Getenv("TEST_DATABASE_URL")

	setupOnce    sync.Once
	setupErr     error
	adminPool    *pgxpool.Pool
	templateName string
)

func ident(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

func randomName(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)

	return fmt.Sprintf("%s%x", prefix, b)
}

func poolFor(ctx context.Context, dbName string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		return nil, fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
	}
	cfg.ConnConfig.Database = dbName

	return pgxpool.NewWithConfig(ctx, cfg)
}

func createTemplate(ctx context.Context) error {
	var err error
	adminPool, err = pgxpool.New(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connect admin pool: %w", err)
	}

	templateName = randomName("test_tmpl_")
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+ident(templateName)); err != nil {
		return fmt.Errorf("create template database %s: %w", templateName, err)
	}

	tmpl, err := poolFor(ctx, templateName)
	if err != nil {
		return fmt.Errorf("connect to template database %s: %w", templateName, err)
	}
	defer tmpl.Close()

	if err := db.Migrate(ctx, tmpl, slog.New(slog.DiscardHandler)); err != nil {
		return fmt.Errorf("migrate template: %w", err)
	}

	return nil
}

func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	setupOnce.Do(func() { setupErr = createTemplate(ctx) })
	if setupErr != nil {
		t.Fatalf("set up template database: %v", setupErr)
	}

	name := randomName("test_")
	_, err := adminPool.Exec(ctx, "CREATE DATABASE "+ident(name)+" TEMPLATE "+ident(templateName))
	if err != nil {
		t.Fatalf("create test database %s from template %s: %v", name, templateName, err)
	}

	pool, err := poolFor(ctx, name)
	if err != nil {
		t.Fatalf("connect to test database %s: %v", name, err)
	}

	t.Cleanup(func() {
		pool.Close()
		_, err := adminPool.Exec(context.Background(), "DROP DATABASE "+ident(name)+" WITH (FORCE)")
		if err != nil {
			t.Errorf("drop test database %s: %v", name, err)
		}
	})

	return pool
}

func Main(m *testing.M) int {
	code := m.Run()
	if adminPool != nil {
		_, err := adminPool.Exec(context.Background(), "DROP DATABASE "+ident(templateName)+" WITH (FORCE)")
		if err != nil {
			fmt.Fprintf(os.Stderr, "testdb: drop template database %s: %v\n", templateName, err)
		}
		adminPool.Close()
	}
	return code
}
