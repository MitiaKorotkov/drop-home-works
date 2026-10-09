package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/MitiaKorotkov/drop-home-works/internal/config"
)

func run(ctx context.Context) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(os.Stderr, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)

	logger.Info("config loaded", "addr", cfg.HTTPAddress, "storage_dir", cfg.StorageDir)

	return nil
}

func main() {
	err := run(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Server:", err)
		os.Exit(1)
	}
}
