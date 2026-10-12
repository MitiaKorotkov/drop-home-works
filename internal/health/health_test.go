package health

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

type mockPinger struct {
	err error
}

func (pinger mockPinger) Ping(ctx context.Context) error {
	return pinger.err
}

type slowPinger struct{}

func (pinger slowPinger) Ping(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return errors.New("ping was not cancelled")
	}
}

type countingPinger struct {
	calls int
	err   error
}

func (pinger *countingPinger) Ping(ctx context.Context) error {
	pinger.calls++
	return pinger.err
}

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		db         Pinger
		timeout    time.Duration
		wantStatus int
		wantBody   string
		wantLog    string
	}{
		{
			name:       "db ok",
			db:         mockPinger{},
			timeout:    1 * time.Second,
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
			wantLog:    "",
		},
		{
			name:       "db down",
			db:         mockPinger{errors.New("db is down")},
			timeout:    1 * time.Second,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unavailable"}`,
			wantLog:    "db is down",
		},
		{
			name:       "db slow",
			db:         slowPinger{},
			timeout:    10 * time.Millisecond,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unavailable"}`,
			wantLog:    "deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := newTestLogger()
			rr := httptest.NewRecorder()

			Handler(tt.db, tt.timeout, logger).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/health", nil))

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if rr.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
			if rr.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("Content-Type = %q, want %q", rr.Header().Get("Content-Type"), "application/json")
			}
			if rr.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q, want %q", rr.Header().Get("Cache-Control"), "no-store")
			}
			if strings.Contains(rr.Body.String(), "db is down") {
				t.Fatalf("body = %q, must not contain the error text %q", rr.Body.String(), "db is down")
			}
			if (tt.wantLog == "" && buf.Len() != 0) ||
				(tt.wantLog != "" && !strings.Contains(buf.String(), tt.wantLog)) {
				t.Fatalf("log = %q, wantLog = %q (empty wantLog means the log must be empty)", buf.String(), tt.wantLog)
			}
		})
	}
}

func TestHandlerPingsOnEveryRequest(t *testing.T) {
	logger, _ := newTestLogger()
	rr := httptest.NewRecorder()

	db := &countingPinger{}
	h := Handler(db, time.Second, logger)

	if db.calls != 0 {
		t.Fatalf("Ping calls after Handler() = %d, want 0: Ping must not run when the handler is created", db.calls)
	}

	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want %d", rr.Code, http.StatusOK)
	}

	rr = httptest.NewRecorder()
	db.err = errors.New("db is down")

	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("second request (db down): status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
	if db.calls != 2 {
		t.Fatalf("Ping calls = %d, want 2: one per request", db.calls)
	}
}
