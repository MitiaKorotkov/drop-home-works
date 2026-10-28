package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/account"
	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/password"
)

// Новая операция в спецификации без правила доступа должна ронять этот тест,
// а не становиться открытой или закрытой по случайности.
func TestOperationAccessComplete(t *testing.T) {
	iface := reflect.TypeFor[api.StrictServerInterface]()

	operations := make(map[string]bool, iface.NumMethod())
	for i := range iface.NumMethod() {
		name := iface.Method(i).Name
		operations[name] = true
		if _, ok := operationAccess[name]; !ok {
			t.Errorf("operation %s has no rule in operationAccess", name)
		}
	}
	for name := range operationAccess {
		if !operations[name] {
			t.Errorf("operationAccess has a rule for %q, but there is no such operation: a typo or the operation was removed from the spec", name)
		}
	}
}

// Правила доступа сверяются с тем, что обещает спецификация:
//   - security: [] — операция открыта всем;
//   - тег auth — любому вошедшему;
//   - тег student — только ученику;
//   - остальные теги — только учителю.
//
// Вместе с TestRequireAccess это проверяет отказ ученику на каждом эндпоинте
// учителя и учителю на каждом эндпоинте ученика.
func TestOperationAccessMatchesSpec(t *testing.T) {
	names := map[access]string{
		public:      "public",
		anyUser:     "anyUser",
		teacherOnly: "teacherOnly",
		studentOnly: "studentOnly",
	}

	doc, err := api.GetSpec()
	if err != nil {
		t.Fatalf("GetSpec: %v", err)
	}
	// Операции без своего security наследуют общее. Без него они были бы открыты всем.
	if len(doc.Security) == 0 {
		t.Fatal("the spec has no top-level security: operations without their own security are public")
	}

	checked := 0
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			checked++

			want := teacherOnly
			switch {
			case op.Security != nil && len(*op.Security) == 0:
				want = public
			case slices.Contains(op.Tags, "auth"):
				want = anyUser
			case slices.Contains(op.Tags, "student"):
				want = studentOnly
			}

			got, ok := operationAccess[op.OperationID]
			if !ok {
				t.Errorf("%s %s: operation %s has no rule in operationAccess", method, path, op.OperationID)
				continue
			}
			if got != want {
				t.Errorf("%s %s: operation %s is %s, the spec says %s", method, path, op.OperationID, names[got], names[want])
			}
		}
	}
	if checked != len(operationAccess) {
		t.Errorf("checked %d operations from the spec, operationAccess has %d rules", checked, len(operationAccess))
	}
}

// Логика проверяется без базы и без HTTP-сервера: middleware вызывается
// напрямую, сессия кладётся в контекст так же, как это делает loadSession.
func TestRequireAccess(t *testing.T) {
	teacher := &currentSession{user: account.User{ID: 1, Role: db.UserRoleTeacher}}
	student := &currentSession{user: account.User{ID: 2, Role: db.UserRoleStudent}}

	tests := []struct {
		name       string
		operation  string
		session    *currentSession
		wantStatus int // 0 — запрос должен дойти до обработчика
		wantCode   api.ErrorCode
	}{
		{"public, no session", "Login", nil, 0, ""},
		{"public, teacher", "Login", teacher, 0, ""},
		{"any user, no session", "GetSession", nil, http.StatusUnauthorized, api.ErrorCodeUnauthorized},
		{"any user, teacher", "GetSession", teacher, 0, ""},
		{"any user, student", "GetSession", student, 0, ""},
		{"teacher only, no session", "ListClasses", nil, http.StatusUnauthorized, api.ErrorCodeUnauthorized},
		{"teacher only, teacher", "ListClasses", teacher, 0, ""},
		{"teacher only, student", "ListClasses", student, http.StatusForbidden, api.ErrorCodeForbidden},
		{"student only, no session", "ListMyAssignments", nil, http.StatusUnauthorized, api.ErrorCodeUnauthorized},
		{"student only, student", "ListMyAssignments", student, 0, ""},
		{"student only, teacher", "ListMyAssignments", teacher, http.StatusForbidden, api.ErrorCodeForbidden},
		{"operation without a rule", "NoSuchOperation", teacher, http.StatusInternalServerError, api.ErrorCodeInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := newTestLogger()
			s := NewServer(logger, nil, DefaultLoginLimits())

			called := false
			next := func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
				called = true
				return "handler response", nil
			}

			ctx := t.Context()
			if tt.session != nil {
				ctx = withSession(ctx, *tt.session)
			}
			rr := httptest.NewRecorder()
			resp, err := s.requireAccess(next, tt.operation)(ctx, rr, httptest.NewRequest(http.MethodGet, "/", nil), nil)
			if err != nil {
				t.Fatalf("requireAccess returned error %v, want nil", err)
			}

			if tt.wantStatus == 0 {
				if !called {
					t.Fatalf("handler was not called; response: %d %s", rr.Code, rr.Body)
				}
				if resp != "handler response" {
					t.Errorf("response = %v, want the handler's response passed through unchanged", resp)
				}
				return
			}

			if called {
				t.Errorf("handler was called, want the request stopped with %d", tt.wantStatus)
			}
			if resp != nil {
				t.Errorf("response = %v, want nil: the generated code would write a second response", resp)
			}
			requireProblem(t, rr.Result(), tt.wantStatus, tt.wantCode)
		})
	}
}

// Сквозная проверка через Register: requireAccess подключён и видит сессию,
// которую положил loadSession. Сама логика ролей проверена в TestRequireAccess.
func TestAccessThroughServer(t *testing.T) {
	t.Run("no cookie on a teacher endpoint", func(t *testing.T) {
		h, _ := newTestServer(t)

		resp := doRequest(t, h, http.MethodGet, "/api/classes", nil)

		requireProblem(t, resp, http.StatusUnauthorized, api.ErrorCodeUnauthorized)
	})

	t.Run("teacher on a student endpoint", func(t *testing.T) {
		h, _, cookie, _ := loginTeacher(t)

		resp := doRequest(t, h, http.MethodGet, "/api/me/assignments", cookie)

		requireProblem(t, resp, http.StatusForbidden, api.ErrorCodeForbidden)
	})

	t.Run("student on a teacher endpoint", func(t *testing.T) {
		h, pool := newTestServer(t)
		q := db.New(pool)
		class, err := q.CreateClass(t.Context(), "7А")
		if err != nil {
			t.Fatalf("CreateClass: %v", err)
		}
		var studentID int64
		err = pool.QueryRow(t.Context(),
			`INSERT INTO users (role, login, password_hash, last_name, first_name, class_id)
			VALUES ('student', 'student', $1, 'Петров', 'Пётр', $2)
			RETURNING id`,
			password.Hash("Student123!"), class.ID,
		).Scan(&studentID)
		if err != nil {
			t.Fatalf("insert student: %v", err)
		}
		login := postLogin(t, h, "student", "Student123!")
		requireLoggedIn(t, q, login, studentID)

		resp := doRequest(t, h, http.MethodGet, "/api/classes", login.Cookies()[0])

		requireProblem(t, resp, http.StatusForbidden, api.ErrorCodeForbidden)
	})
}
