package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/httpserver"
)

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   api.ErrorCode
	}{
		{"session without login", http.MethodGet, "/api/session", "", http.StatusUnauthorized, api.ErrorCodeUnauthorized},
		{"broken JSON body", http.MethodPost, "/api/session", `{"login":`, http.StatusBadRequest, api.ErrorCodeValidationFailed},
		{"invalid login format", http.MethodPost, "/api/session", `{"login":"a","password":"b"}`, http.StatusUnauthorized, api.ErrorCodeInvalidCredentials},
		{"bad path parameter", http.MethodGet, "/api/assignments/abc", "", http.StatusBadRequest, api.ErrorCodeValidationFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := newTestLogger()
			mux := http.NewServeMux()
			Register(mux, NewServer(logger, nil, DefaultLoginLimits()))
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			httpserver.New(mux, logger).Handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want status = %d", rr.Code, tt.wantStatus)
			}
			if rr.Header().Get("Content-Type") != "application/problem+json" {
				t.Errorf("Content-Type = %q, want %q", rr.Header().Get("Content-Type"), "application/problem+json")
			}
			var p api.Problem
			if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
				t.Fatalf("Error when unmarshal response body: %v", err)
			}
			if p.Code != tt.wantCode || p.Status != int32(tt.wantStatus) {
				t.Errorf("code = %q, want code = %q; status = %d, want status = %d", p.Code, tt.wantCode, p.Status, int32(tt.wantStatus))
			}
		})
	}
}
