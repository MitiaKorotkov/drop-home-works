package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newLogger(w io.Writer, format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler = slog.NewTextHandler(w, opts)
	if format == "json" {
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(handler)
}

// NOTE: подключается к базе и применяет миграции.
// Пул закрывает вызывающий
func openDB(ctx context.Context, url string, logger *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("create db pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := db.Migrate(ctx, pool, logger); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return pool, nil
}
