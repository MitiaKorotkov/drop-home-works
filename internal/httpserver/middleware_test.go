package httpserver

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func catchPanic(f func()) (v any) {
	defer func() { v = recover() }()
	f()
	return nil
}

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestLogRequests(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
	}{
		{
			name:       "explicit 201",
			handler:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) },
			wantStatus: http.StatusCreated,
		},
		{
			name:       "body without WriteHeader",
			handler:    func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("test")) },
			wantStatus: http.StatusOK,
		},
		{
			name:       "nothing written",
			handler:    func(w http.ResponseWriter, r *http.Request) {},
			wantStatus: http.StatusOK,
		},
		{
			name: "double WriteHeader keeps first",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.WriteHeader(http.StatusBadRequest)
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := newTestLogger()
			rr := httptest.NewRecorder()

			logRequests(logger, tt.handler).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/some/path", nil))

			for _, want := range []string{
				"msg=request",
				"method=GET",
				"path=/some/path",
				fmt.Sprintf("status=%d", tt.wantStatus),
			} {
				if !strings.Contains(buf.String(), want) {
					t.Errorf("log = %q, want it to contain %q", buf.String(), want)
				}
			}

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want status = %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestRecoverPanics(t *testing.T) {
	t.Run("no panic", func(t *testing.T) {
		logger, buf := newTestLogger()
		rr := httptest.NewRecorder()

		var h http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("ok"))
		}
		recoverPanics(logger, h).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/some/path", nil))

		if rr.Code != http.StatusCreated {
			t.Fatalf("status = %d, want status = 201", rr.Code)
		}
		if rr.Body.String() != "ok" {
			t.Fatalf("body = %q, want body = ok", rr.Body.String())
		}
		if buf.Len() != 0 {
			t.Fatalf("log = %q, want empty log", buf.String())
		}
	})

	t.Run("panic before write", func(t *testing.T) {
		logger, buf := newTestLogger()
		rr := httptest.NewRecorder()

		var h http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			panic("boom!")
		}
		recoverPanics(logger, h).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/some/path", nil))

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want status = 500", rr.Code)
		}
		if rr.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("Content-Type = %q, want %q", rr.Header().Get("Content-Type"), "application/problem+json")
		}
		if !strings.Contains(rr.Body.String(), `"code":"internal_error"`) {
			t.Errorf("body = %q, want it to contain %q", rr.Body.String(), `"code":"internal_error"`)
		}
		for _, want := range []string{
			"level=ERROR",
			"value=boom!",
			"path=/some/path",
		} {
			if !strings.Contains(buf.String(), want) {
				t.Errorf("log = %q, want it to contain %q", buf.String(), want)
			}
		}
	})

	t.Run("panic after write", func(t *testing.T) {
		logger, buf := newTestLogger()
		rr := httptest.NewRecorder()

		var h http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("half"))
			panic("boom!")
		}
		v := catchPanic(func() {
			recoverPanics(logger, h).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/some/path", nil))
		})

		if v != http.ErrAbortHandler {
			t.Errorf("panic value = %v, want http.ErrAbortHandler", v)
		}
		if !strings.Contains(buf.String(), "level=ERROR") {
			t.Errorf("log = %q, want it to contain %q", buf.String(), "level=ERROR")
		}
	})

	t.Run("ErrAbortHandler", func(t *testing.T) {
		logger, buf := newTestLogger()
		rr := httptest.NewRecorder()

		var h http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			panic(http.ErrAbortHandler)
		}
		v := catchPanic(func() {
			recoverPanics(logger, h).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/some/path", nil))
		})

		if v != http.ErrAbortHandler {
			t.Errorf("panic value = %v, want http.ErrAbortHandler", v)
		}
		if buf.Len() != 0 {
			t.Errorf("log = %q, want empty log", buf.String())
		}
	})

	t.Run("stale headers", func(t *testing.T) {
		logger, _ := newTestLogger()
		rr := httptest.NewRecorder()

		var h http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Disposition", `attachment; filename="work.pdf"`)
			w.Header().Set("Content-Length", "20000000")
			panic("boom!")
		}
		recoverPanics(logger, h).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/some/path", nil))

		if rr.Header().Get("Content-Disposition") != "" {
			t.Errorf("Content-Disposition = %q, want empty", rr.Header().Get("Content-Disposition"))
		}
		if rr.Header().Get("Content-Length") != "" {
			t.Errorf("Content-Length = %q, want empty", rr.Header().Get("Content-Length"))
		}
		if rr.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("Content-Type = %q, want %q", rr.Header().Get("Content-Type"), "application/problem+json")
		}
	})
}

func TestNew(t *testing.T) {
	logger, buf := newTestLogger()
	rr := httptest.NewRecorder()
	var h http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		panic("boom!")
	}

	srv := New(h, logger)
	srv.Handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/some/path", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want status = 500", rr.Code)
	}
	for _, want := range []string{
		"level=ERROR",
		"status=500",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log = %q, want it to contain %q", buf.String(), want)
		}
	}
}
