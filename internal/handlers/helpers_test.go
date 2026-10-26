package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/account"
	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/httpserver"
	"github.com/MitiaKorotkov/drop-home-works/internal/session"
	"github.com/MitiaKorotkov/drop-home-works/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testTeacher = account.TeacherInput{
	Login:     "teacher",
	Password:  "Teacher123!",
	FirstName: "Иван",
	LastName:  "Иванов",
}

func postLogin(t *testing.T, h http.Handler, login, passwd string) *http.Response {
	t.Helper()
	return postLoginFrom(t, h, "", login, passwd)
}

// postLoginFrom отправляет запрос входа с адреса remoteAddr вида "ip:port".
// Пустая строка — адрес httptest по умолчанию, 192.0.2.1:1234.
func postLoginFrom(t *testing.T, h http.Handler, remoteAddr, login, passwd string) *http.Response {
	t.Helper()

	body, err := json.Marshal(api.LoginRequest{Login: login, Password: passwd})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(body))
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Result()
}

func newTestServer(t *testing.T) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	return newTestServerWithLimits(t, DefaultLoginLimits())
}

func newTestServerWithLimits(t *testing.T, limits LoginLimits) (http.Handler, *pgxpool.Pool) {
	t.Helper()

	logger, _ := newTestLogger()
	pool := testdb.New(t)
	s := NewServer(logger, db.New(pool), limits)
	mux := http.NewServeMux()

	Register(mux, s)

	return httpserver.New(mux, logger).Handler, pool
}

func requireLoggedIn(t *testing.T, q *db.Queries, resp *http.Response, wantUserID int64) api.Me {
	t.Helper()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var me api.Me
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if me.Id != wantUserID {
		t.Errorf("response user id = %d, want %d", me.Id, wantUserID)
	}

	cookies := resp.Cookies()
	if len(cookies) != 1 || cookies[0].Name != session.CookieName {
		t.Fatalf("Set-Cookie = %q, want exactly one %q cookie", resp.Header.Values("Set-Cookie"), session.CookieName)
	}
	if !cookies[0].HttpOnly {
		t.Errorf("session cookie is not HttpOnly: scripts on the page can read the token")
	}

	u, err := q.GetSessionUser(t.Context(), session.HashToken(cookies[0].Value))
	if err != nil {
		t.Fatalf("GetSessionUser(hash of the cookie token): %v: the cookie does not lead to a session in the database", err)
	}
	if u.ID != wantUserID {
		t.Errorf("session belongs to user %d, want %d", u.ID, wantUserID)
	}

	return me
}

func requireProblem(t *testing.T, resp *http.Response, wantStatus int, wantCode api.ErrorCode) []byte {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != wantStatus {
		t.Errorf("status = %d, want %d; body: %s", resp.StatusCode, wantStatus, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/problem+json")
	}

	var p api.Problem
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("decode problem %s: %v", body, err)
	}
	if p.Code != wantCode {
		t.Errorf("code = %q, want %q", p.Code, wantCode)
	}

	return body
}

func doRequest(t *testing.T, h http.Handler, method, target string, cookie *http.Cookie) *http.Response {
	t.Helper()

	req := httptest.NewRequest(method, target, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	return rr.Result()
}

func loginTeacher(t *testing.T) (http.Handler, *pgxpool.Pool, *http.Cookie, api.Me) {
	t.Helper()

	ctx := t.Context()
	h, pool := newTestServer(t)
	q := db.New(pool)

	teacherID, err := account.CreateTeacher(ctx, q, testTeacher)
	if err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}

	resp := postLogin(t, h, testTeacher.Login, testTeacher.Password)
	me := requireLoggedIn(t, q, resp, teacherID)
	cookie := resp.Cookies()[0]

	return h, pool, cookie, me
}

func requireLoggedOut(t *testing.T, resp *http.Response) {
	t.Helper()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusNoContent, body)
	}

	cookies := resp.Cookies()
	if len(cookies) != 1 || cookies[0].Name != session.CookieName {
		t.Fatalf("Set-Cookie = %q, want exactly one %q cookie", resp.Header.Values("Set-Cookie"), session.CookieName)
	}
	if cookies[0].MaxAge >= 0 {
		t.Fatalf("cookie max-age = %d, want < 0", cookies[0].MaxAge)
	}
}
