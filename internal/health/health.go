package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

func Handler(db Pinger, timeout time.Duration, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), timeout)
		defer cancel()

		status, body := http.StatusOK, `{"status":"ok"}`
		if err := db.Ping(ctx); err != nil {
			logger.Warn("db ping failed", "error", err)

			status = http.StatusServiceUnavailable
			body = `{"status":"unavailable"}`
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		w.Write([]byte(body))
	})
}
