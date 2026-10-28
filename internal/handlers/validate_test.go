package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/session"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// Проверка по спецификации стоит раньше loadSession и requireAccess, поэтому
// база этим тестам не нужна. Тело с ошибкой получает 400, а правильное доходит
// до requireAccess и без cookie получает 401: так видно, что проверка его пропустила.
func TestValidateRequest(t *testing.T) {
	const (
		jsonType      = "application/json"
		multipartType = "multipart/form-data; boundary=x"
		student       = `"firstName":"Пётр","lastName":"Петров","login":"petrov","password":"Student123!"`
	)

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		wantStatus  int
		wantCode    api.ErrorCode
	}{
		{"required property is missing", http.MethodPost, "/api/students", jsonType, `{` + student + `}`, http.StatusBadRequest, api.ErrorCodeValidationFailed},
		{"all required properties", http.MethodPost, "/api/students", jsonType, `{"classId":1,` + student + `}`, http.StatusUnauthorized, api.ErrorCodeUnauthorized},
		{"login does not match the pattern", http.MethodPost, "/api/students", jsonType, `{"classId":1,` + strings.Replace(student, `"petrov"`, `"ab"`, 1) + `}`, http.StatusBadRequest, api.ErrorCodeValidationFailed},
		{"empty update", http.MethodPatch, "/api/students/1", jsonType, `{}`, http.StatusBadRequest, api.ErrorCodeValidationFailed},
		{"partial update", http.MethodPatch, "/api/students/1", jsonType, `{"firstName":"Пётр"}`, http.StatusUnauthorized, api.ErrorCodeUnauthorized},
		{"wrong property type", http.MethodPost, "/api/classes", jsonType, `{"name":7}`, http.StatusBadRequest, api.ErrorCodeValidationFailed},
		{"no Content-Type", http.MethodPost, "/api/classes", "", `{"name":"7А"}`, http.StatusBadRequest, api.ErrorCodeValidationFailed},
		// Такой Content-Type ставит fetch, если тело — строка, а заголовок не задан.
		{"fetch default Content-Type", http.MethodPost, "/api/classes", "text/plain;charset=UTF-8", `{"name":"7А"}`, http.StatusBadRequest, api.ErrorCodeValidationFailed},
		// Тело загрузок фильтр не читает: иначе файл целиком оказался бы в памяти.
		// Мусор вместо multipart доходит до requireAccess.
		{"submission upload body is not read", http.MethodPut, "/api/me/assignments/1/submission", multipartType, "garbage", http.StatusUnauthorized, api.ErrorCodeUnauthorized},
		{"assignment file upload body is not read", http.MethodPost, "/api/assignments/1/files", multipartType, "garbage", http.StatusUnauthorized, api.ErrorCodeUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newTestServerWithoutDB(t)

			resp := doRaw(t, h, tt.method, tt.path, tt.contentType, tt.body)

			requireProblem(t, resp, tt.wantStatus, tt.wantCode)
		})
	}
}

// По умолчанию kin-openapi дописывает к ошибке проверяемое значение,
// и пароль попал бы в ответ.
func TestValidateRequestHidesValues(t *testing.T) {
	const secret = "Secret7"

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"student password is too short", http.MethodPut, "/api/students/1/password", `{"password":"` + secret + `"}`},
		{"login password is too long", http.MethodPost, "/api/session", `{"login":"petrov","password":"` + strings.Repeat(secret, 19) + `"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newTestServerWithoutDB(t)

			resp := doRaw(t, h, tt.method, tt.path, "application/json", tt.body)

			body := requireProblem(t, resp, http.StatusBadRequest, api.ErrorCodeValidationFailed)
			if strings.Contains(string(body), secret) {
				t.Errorf("response contains the password: %s", body)
			}
			var p api.Problem
			if err := json.Unmarshal(body, &p); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if p.Detail == nil || !strings.Contains(*p.Detail, "/password") {
				t.Errorf("detail = %v, want it to name the /password property", p.Detail)
			}
		})
	}
}

func TestValidateRequestOrder(t *testing.T) {
	const badBody = `{"name":7}`

	// Без базы loadSession с cookie запаниковал бы, и ответ был бы 500.
	t.Run("before the session lookup", func(t *testing.T) {
		h, _ := newTestServerWithoutDB(t)
		req := httptest.NewRequest(http.MethodPost, "/api/classes", strings.NewReader(badBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: "some-token"})
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		requireProblem(t, rr.Result(), http.StatusBadRequest, api.ErrorCodeValidationFailed)
	})

	t.Run("after the cross-origin check", func(t *testing.T) {
		h, _ := newTestServerWithoutDB(t)
		req := httptest.NewRequest(http.MethodPost, "/api/classes", strings.NewReader(badBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rr := httptest.NewRecorder()

		h.ServeHTTP(rr, req)

		requireProblem(t, rr.Result(), http.StatusForbidden, api.ErrorCodeCrossOrigin)
	})
}

// Маршрут, которого нет в спецификации, через Register до проверки не дойдёт:
// его раньше отсечёт ServeMux. Поэтому middleware вызывается напрямую.
// Такой запрос нельзя пропускать дальше непроверенным.
func TestValidateRequestUnknownRoute(t *testing.T) {
	doc, err := api.GetSpec()
	if err != nil {
		t.Fatalf("GetSpec: %v", err)
	}
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	logger, logs := newTestLogger()
	s := NewServer(logger, nil, DefaultLoginLimits())
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	rr := httptest.NewRecorder()

	s.validateRequest(router)(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/no-such-route", nil))

	requireProblem(t, rr.Result(), http.StatusInternalServerError, api.ErrorCodeInternalError)
	if called {
		t.Error("request without a route in the spec was passed to the next handler")
	}
	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("no error in the log: %q", logs.String())
	}
}
