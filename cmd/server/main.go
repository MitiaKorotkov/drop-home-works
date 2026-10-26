package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MitiaKorotkov/drop-home-works/internal/config"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/handlers"
	"github.com/MitiaKorotkov/drop-home-works/internal/health"
	"github.com/MitiaKorotkov/drop-home-works/internal/httpserver"
)

func run(ctx context.Context) error {
	// 0. Context
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Config
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 2. Logger
	logger := newLogger(os.Stderr, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("config loaded", "addr", cfg.HTTPAddress, "storage_dir", cfg.StorageDir)

	// 3. Pgx pool
	pool, err := openDB(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	// 4. Port
	ln, err := net.Listen("tcp", cfg.HTTPAddress)
	if err != nil {
		return fmt.Errorf("open port: %w", err)
	}

	// 5. Server
	mux := http.NewServeMux()
	mux.Handle("GET /api/health", health.Handler(pool, 2*time.Second, logger))

	limits := handlers.DefaultLoginLimits()
	go limits.ByIP.Run(ctx, time.Minute)
	go limits.ByIPLogin.Run(ctx, time.Minute)

	handlers.Register(mux, handlers.NewServer(logger, db.New(pool), limits))

	srv := httpserver.New(mux, logger)

	// 6. Run
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()
	logger.Info("listening", "addr", ln.Addr().String())

	select {
	case err := <-errCh:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
		// NOTE: Переходим к остановке
	}

	// 7. Stop
	stop()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("server stopped")

	return nil
}

func main() {
	ctx := context.Background()
	if len(os.Args) > 1 && os.Args[1] == "create-teacher" {
		if err := runCreateTeacher(ctx, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "create teacher:", err)
			os.Exit(1)
		}
	} else {
		if err := run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "server:", err)
			os.Exit(1)
		}
	}
}
