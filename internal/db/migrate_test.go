package db_test

import (
	"context"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/testdb"
	"github.com/MitiaKorotkov/drop-home-works/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func countTables(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()

	var n int64
	err := pool.QueryRow(
		context.Background(),
		`SELECT count(*)
		FROM pg_catalog.pg_tables
		WHERE schemaname = current_schema()`,
	).Scan(&n)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}

	return n
}

func TestMigrate(t *testing.T) {
	pool := testdb.New(t)

	if n := countTables(t, pool); n != 9 {
		t.Fatalf("tables after migrations = %d, want 9 (8 from 00001_init.sql + goose_db_version)", n)
	}
}

func TestMigrateDownUp(t *testing.T) {
	ctx := context.Background()

	pool := testdb.New(t)
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatalf("create goose provider: %v", err)
	}

	_, err = provider.DownTo(ctx, 0)
	if err != nil {
		t.Fatalf("goose down to version 0: %v", err)
	}
	if n := countTables(t, pool); n != 1 {
		t.Fatalf("tables after down = %d, want 1 (only goose_db_version): the Down section leaves tables behind", n)
	}

	_, err = provider.Up(ctx)
	if err != nil {
		t.Fatalf("goose up after down: %v (the Down section probably leaves a type, function or trigger behind)", err)
	}
	if n := countTables(t, pool); n != 9 {
		t.Fatalf("tables after down and up = %d, want 9", n)
	}
}
