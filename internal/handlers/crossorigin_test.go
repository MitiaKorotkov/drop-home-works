package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/httpserver"
	"github.com/MitiaKorotkov/drop-home-works/internal/session"
)

func TestCrossOriginProtection(t *testing.T) {
	// Пропущенный запрос входа с логином неверного формата получает 401 ещё до
	// обращения к базе, поэтому база тесту не нужна. Отклонённый — 403 cross_origin.
	const loginBody = `{"login":"a","password":"b"}`

	tests := []struct {
		name         string
		method       string
		secFetchSite string
		origin       string
		withCookie   bool
		wantStatus   int
		wantCode     api.ErrorCode
	}{
		{"cross-site", http.MethodPost, "cross-site", "", false, http.StatusForbidden, api.ErrorCodeCrossOrigin},
		{"same-site: a sibling subdomain", http.MethodPost, "same-site", "", false, http.StatusForbidden, api.ErrorCodeCrossOrigin},
		{"same-origin", http.MethodPost, "same-origin", "", false, http.StatusUnauthorized, api.ErrorCodeInvalidCredentials},
		{"none: typed address or bookmark", http.MethodPost, "none", "", false, http.StatusUnauthorized, api.ErrorCodeInvalidCredentials},
		{"no Sec-Fetch-Site, foreign Origin", http.MethodPost, "", "https://evil.example", false, http.StatusForbidden, api.ErrorCodeCrossOrigin},
		{"no Sec-Fetch-Site, own Origin", http.MethodPost, "", "https://school.example", false, http.StatusUnauthorized, api.ErrorCodeInvalidCredentials},
		{"no headers: curl or another non-browser client", http.MethodPost, "", "", false, http.StatusUnauthorized, api.ErrorCodeInvalidCredentials},
		{"GET is a safe method and is not checked", http.MethodGet, "cross-site", "", false, http.StatusUnauthorized, api.ErrorCodeUnauthorized},
		{"logout from another site", http.MethodDelete, "cross-site", "", false, http.StatusForbidden, api.ErrorCodeCrossOrigin},
		// Без базы loadSession с cookie запаниковал бы, и ответ был бы 500.
		// 403 значит, что запрос отклонён раньше, чем дошёл до поиска сессии.
		{"rejected before the session lookup", http.MethodPost, "cross-site", "", true, http.StatusForbidden, api.ErrorCodeCrossOrigin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := newTestLogger()
			mux := http.NewServeMux()
			Register(mux, NewServer(logger, nil, DefaultLoginLimits()))
			h := httpserver.New(mux, logger).Handler

			var body io.Reader
			if tt.method == http.MethodPost {
				body = strings.NewReader(loginBody)
			}
			// Абсолютный URL задаёт заголовок Host: school.example.
			req := httptest.NewRequest(tt.method, "https://school.example/api/session", body)
			if tt.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.secFetchSite)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.withCookie {
				req.AddCookie(&http.Cookie{Name: session.CookieName, Value: "some-token"})
			}

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			requireProblem(t, rr.Result(), tt.wantStatus, tt.wantCode)
		})
	}
}
