package httpserver

import (
	"log/slog"
	"net/http"
	"time"
)

const (
	timeoutRead       = 30 * time.Second
	timeoutWrite      = 60 * time.Second
	timeoutReadHeader = 10 * time.Second
	timeoutIdle       = 120 * time.Second
)

func New(handler http.Handler, logger *slog.Logger) *http.Server {
	server := &http.Server{
		Handler:           logRequests(logger, recoverPanics(logger, handler)),
		ReadTimeout:       timeoutRead,
		WriteTimeout:      timeoutWrite,
		ReadHeaderTimeout: timeoutReadHeader,
		IdleTimeout:       timeoutIdle,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	return server
}
