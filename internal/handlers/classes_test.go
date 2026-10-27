package handlers

import (
	"bytes"
	"cmp"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/testdb"
)

// Доступ только для учителя проверен в TestRequireAccess и TestAccessThroughServer,
// разбор битого JSON — в TestRegister. Здесь — логика самих обработчиков.

func TestCreateClass(t *testing.T) {
	t.Run("created", func(t *testing.T) {
		h, pool, cookie, _ := loginTeacher(t)

		resp := doJSON(t, h, http.MethodPost, "/api/classes", api.ClassCreate{Name: "  7А "}, cookie)

		var got api.Class
		requireJSON(t, resp, http.StatusCreated, &got)
		if got.Id <= 0 || got.Name != "7А" {
			t.Errorf("response = %+v, want id > 0 and name %q with the spaces trimmed", got, "7А")
		}

		rows, err := db.New(pool).ListClasses(t.Context())
		if err != nil {
			t.Fatalf("ListClasses: %v", err)
		}
		want := []db.ListClassesRow{{ID: got.Id, Name: "7А"}}
		if !slices.Equal(rows, want) {
			t.Errorf("classes in the database = %+v, want %+v", rows, want)
		}
	})

	t.Run("name taken", func(t *testing.T) {
		h, _, cookie, _ := loginTeacher(t)
		first := doJSON(t, h, http.MethodPost, "/api/classes", api.ClassCreate{Name: "7А"}, cookie)
		requireJSON(t, first, http.StatusCreated, &api.Class{})

		resp := doJSON(t, h, http.MethodPost, "/api/classes", api.ClassCreate{Name: "7А"}, cookie)

		requireProblem(t, resp, http.StatusConflict, api.ErrorCodeClassNameTaken)
	})

	t.Run("invalid name", func(t *testing.T) {
		// Ни один запрос ничего не создаёт, поэтому сервер у всех случаев общий.
		h, _, cookie, _ := loginTeacher(t)

		for _, tt := range []struct {
			name string
			body any
		}{
			{"empty", api.ClassCreate{Name: ""}},
			{"only spaces", api.ClassCreate{Name: "   "}},
			{"too long", api.ClassCreate{Name: strings.Repeat("я", 31)}},
			{"no name field", map[string]any{}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				resp := doJSON(t, h, http.MethodPost, "/api/classes", tt.body, cookie)

				requireProblem(t, resp, http.StatusBadRequest, api.ErrorCodeValidationFailed)
			})
		}
	})
}

func TestListClasses(t *testing.T) {
	t.Run("no classes", func(t *testing.T) {
		h, _, cookie, _ := loginTeacher(t)

		resp := doRequest(t, h, http.MethodGet, "/api/classes", cookie)

		var got []api.Class
		body := requireJSON(t, resp, http.StatusOK, &got)
		// И [], и null разбираются в пустой срез, поэтому проверяется сам текст.
		if s := string(bytes.TrimSpace(body)); s != "[]" {
			t.Errorf("body = %s, want []: null instead of an empty list breaks the frontend", s)
		}
	})

	t.Run("all classes", func(t *testing.T) {
		h, pool, cookie, _ := loginTeacher(t)
		q := db.New(pool)
		var want []api.Class
		for _, name := range []string{"7Б", "11А", "7А"} {
			c, err := q.CreateClass(t.Context(), name)
			if err != nil {
				t.Fatalf("CreateClass(%q): %v", name, err)
			}
			want = append(want, api.Class{Id: c.ID, Name: c.Name})
		}

		var got []api.Class
		requireJSON(t, doRequest(t, h, http.MethodGet, "/api/classes", cookie), http.StatusOK, &got)

		// Порядок не проверяется: классы сортирует фронтенд.
		slices.SortFunc(got, func(a, b api.Class) int { return cmp.Compare(a.Id, b.Id) })
		if !slices.Equal(got, want) {
			t.Errorf("GET /api/classes = %+v, want %+v in any order", got, want)
		}
	})
}

// Через HTTP ошибку базы в этих обработчиках не получить: с закрытым пулом
// раньше упадёт поиск сессии в loadSession. Поэтому они вызываются напрямую.
func TestClassesDatabaseError(t *testing.T) {
	logger, _ := newTestLogger()
	pool := testdb.New(t)
	s := NewServer(logger, db.New(pool), DefaultLoginLimits())
	pool.Close()

	if resp, err := s.ListClasses(t.Context(), api.ListClassesRequestObject{}); err == nil {
		t.Errorf("ListClasses with the database closed = %+v, nil; want an error, so that the client gets 500", resp)
	}
	create := api.CreateClassRequestObject{Body: &api.ClassCreate{Name: "7А"}}
	if resp, err := s.CreateClass(t.Context(), create); err == nil {
		t.Errorf("CreateClass with the database closed = %+v, nil; want an error, so that the client gets 500", resp)
	}
}
