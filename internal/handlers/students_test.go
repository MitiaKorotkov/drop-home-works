package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/testdb"
)

// Проверка ввода и разбор ошибок базы проверены в account. Здесь — что
// обработчики превращают их в правильные ответы.

func TestCreateStudent(t *testing.T) {
	t.Run("created", func(t *testing.T) {
		h, pool, cookie, _ := loginTeacher(t)
		q := db.New(pool)
		class, err := q.CreateClass(t.Context(), "7А")
		if err != nil {
			t.Fatalf("CreateClass: %v", err)
		}

		resp := doJSON(t, h, http.MethodPost, "/api/students", api.StudentCreate{
			ClassId:   class.ID,
			FirstName: " Пётр ",
			LastName:  "Петров",
			Login:     "Petrov",
			Password:  "Student123!",
		}, cookie)

		var got api.Student
		requireJSON(t, resp, http.StatusCreated, &got)
		want := api.Student{Id: got.Id, ClassId: class.ID, FirstName: "Пётр", LastName: "Петров", Login: "petrov"}
		if got.Id <= 0 || got != want {
			t.Errorf("response = %+v, want %+v with id > 0", got, want)
		}

		// Ученик входит с логином и паролем, которые задал учитель.
		requireLoggedIn(t, q, postLogin(t, h, "petrov", "Student123!"), got.Id)
	})

	t.Run("rejected", func(t *testing.T) {
		// Ни один запрос ничего не создаёт, поэтому сервер у всех случаев общий.
		h, pool, cookie, _ := loginTeacher(t)
		q := db.New(pool)
		class, err := q.CreateClass(t.Context(), "7А")
		if err != nil {
			t.Fatalf("CreateClass: %v", err)
		}
		valid := api.StudentCreate{
			ClassId:   class.ID,
			FirstName: "Пётр",
			LastName:  "Петров",
			Login:     "petrov",
			Password:  "Student123!",
		}

		for _, tt := range []struct {
			name       string
			modify     func(b *api.StudentCreate)
			wantStatus int
			wantCode   api.ErrorCode
		}{
			{"bad login", func(b *api.StudentCreate) { b.Login = "ab" }, http.StatusBadRequest, api.ErrorCodeValidationFailed},
			{"short password", func(b *api.StudentCreate) { b.Password = "1234567" }, http.StatusBadRequest, api.ErrorCodeValidationFailed},
			{"unknown class", func(b *api.StudentCreate) { b.ClassId = class.ID + 1000 }, http.StatusBadRequest, api.ErrorCodeClassNotFound},
			{"login of the teacher", func(b *api.StudentCreate) { b.Login = testTeacher.Login }, http.StatusConflict, api.ErrorCodeLoginTaken},
		} {
			t.Run(tt.name, func(t *testing.T) {
				body := valid
				tt.modify(&body)

				resp := doJSON(t, h, http.MethodPost, "/api/students", body, cookie)

				requireProblem(t, resp, tt.wantStatus, tt.wantCode)
			})
		}

		students, err := q.ListClassStudents(t.Context(), class.ID)
		if err != nil {
			t.Fatalf("ListClassStudents: %v", err)
		}
		if len(students) != 0 {
			t.Errorf("students in the class after rejected requests = %+v, want none", students)
		}
	})
}

func TestListClassStudents(t *testing.T) {
	h, pool, cookie, _ := loginTeacher(t)
	q := db.New(pool)

	createClass := func(name string) db.Class {
		t.Helper()
		c, err := q.CreateClass(t.Context(), name)
		if err != nil {
			t.Fatalf("CreateClass(%q): %v", name, err)
		}
		return c
	}
	classA, classB, empty := createClass("7А"), createClass("7Б"), createClass("8А")

	ids := make(map[string]int64)
	for _, s := range []db.CreateStudentParams{
		{Login: "petrov", PasswordHash: "hash", LastName: "Петров", FirstName: "Пётр", ClassID: classA.ID},
		{Login: "abramov", PasswordHash: "hash", LastName: "Абрамов", FirstName: "Олег", ClassID: classB.ID},
		{Login: "ivanov", PasswordHash: "hash", LastName: "Иванов", FirstName: "Сергей", ClassID: classA.ID},
	} {
		row, err := q.CreateStudent(t.Context(), s)
		if err != nil {
			t.Fatalf("CreateStudent(%q): %v", s.Login, err)
		}
		ids[s.Login] = row.ID
	}

	path := func(classID int64) string { return fmt.Sprintf("/api/classes/%d/students", classID) }

	t.Run("students of the class, sorted by name", func(t *testing.T) {
		var got []api.Student
		requireJSON(t, doRequest(t, h, http.MethodGet, path(classA.ID), cookie), http.StatusOK, &got)

		// Ученика из 7Б в ответе нет.
		want := []api.Student{
			{Id: ids["ivanov"], ClassId: classA.ID, FirstName: "Сергей", LastName: "Иванов", Login: "ivanov"},
			{Id: ids["petrov"], ClassId: classA.ID, FirstName: "Пётр", LastName: "Петров", Login: "petrov"},
		}
		if !slices.Equal(got, want) {
			t.Errorf("GET %s = %+v, want %+v", path(classA.ID), got, want)
		}
	})

	t.Run("empty class", func(t *testing.T) {
		var got []api.Student
		body := requireJSON(t, doRequest(t, h, http.MethodGet, path(empty.ID), cookie), http.StatusOK, &got)

		if s := string(bytes.TrimSpace(body)); s != "[]" {
			t.Errorf("body = %s, want []: null instead of an empty list breaks the frontend", s)
		}
	})

	t.Run("unknown class", func(t *testing.T) {
		resp := doRequest(t, h, http.MethodGet, path(classA.ID+1000), cookie)

		requireProblem(t, resp, http.StatusNotFound, api.ErrorCodeNotFound)
	})
}

// Как и в TestClassesDatabaseError: через HTTP с закрытым пулом раньше упадёт loadSession.
func TestStudentsDatabaseError(t *testing.T) {
	logger, _ := newTestLogger()
	pool := testdb.New(t)
	s := NewServer(logger, db.New(pool), DefaultLoginLimits())
	pool.Close()

	list := api.ListClassStudentsRequestObject{ClassId: 1}
	if resp, err := s.ListClassStudents(t.Context(), list); err == nil {
		t.Errorf("ListClassStudents with the database closed = %+v, nil; want an error, so that the client gets 500", resp)
	}
	create := api.CreateStudentRequestObject{Body: &api.StudentCreate{
		ClassId:   1,
		FirstName: "Пётр",
		LastName:  "Петров",
		Login:     "petrov",
		Password:  "Student123!",
	}}
	if resp, err := s.CreateStudent(t.Context(), create); err == nil {
		t.Errorf("CreateStudent with the database closed = %+v, nil; want an error, so that the client gets 500", resp)
	}
}
