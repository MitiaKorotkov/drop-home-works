package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/MitiaKorotkov/drop-home-works/internal/account"
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

	t.Run("students of the class, sorted by name", func(t *testing.T) {
		var got []api.Student
		requireJSON(t, doRequest(t, h, http.MethodGet, classStudentsPath(classA.ID), cookie), http.StatusOK, &got)

		// Ученика из 7Б в ответе нет.
		want := []api.Student{
			{Id: ids["ivanov"], ClassId: classA.ID, FirstName: "Сергей", LastName: "Иванов", Login: "ivanov"},
			{Id: ids["petrov"], ClassId: classA.ID, FirstName: "Пётр", LastName: "Петров", Login: "petrov"},
		}
		if !slices.Equal(got, want) {
			t.Errorf("GET %s = %+v, want %+v", classStudentsPath(classA.ID), got, want)
		}
	})

	t.Run("empty class", func(t *testing.T) {
		var got []api.Student
		body := requireJSON(t, doRequest(t, h, http.MethodGet, classStudentsPath(empty.ID), cookie), http.StatusOK, &got)

		if s := string(bytes.TrimSpace(body)); s != "[]" {
			t.Errorf("body = %s, want []: null instead of an empty list breaks the frontend", s)
		}
	})

	t.Run("unknown class", func(t *testing.T) {
		resp := doRequest(t, h, http.MethodGet, classStudentsPath(classA.ID+1000), cookie)

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
	update := api.UpdateStudentRequestObject{StudentId: 1, Body: &api.StudentUpdate{FirstName: new("Павел")}}
	if resp, err := s.UpdateStudent(t.Context(), update); err == nil {
		t.Errorf("UpdateStudent with the database closed = %+v, nil; want an error, so that the client gets 500", resp)
	}
	if resp, err := s.DeleteStudent(t.Context(), api.DeleteStudentRequestObject{StudentId: 1}); err == nil {
		t.Errorf("DeleteStudent with the database closed = %+v, nil; want an error, so that the client gets 500", resp)
	}
}

const studentPassword = "Student123!"

// studentFixture — сервер с вошедшим учителем, классами 7А и 8А
// и учеником Петровым Петром (логин petrov) в 7А.
type studentFixture struct {
	h              http.Handler
	q              *db.Queries
	teacher        *http.Cookie
	teacherID      int64
	classA, classB db.Class
	student        db.CreateStudentRow
}

func newStudentFixture(t *testing.T) studentFixture {
	t.Helper()

	h, pool, cookie, me := loginTeacher(t)
	q := db.New(pool)
	classA, err := q.CreateClass(t.Context(), "7А")
	if err != nil {
		t.Fatalf("CreateClass(7А): %v", err)
	}
	classB, err := q.CreateClass(t.Context(), "8А")
	if err != nil {
		t.Fatalf("CreateClass(8А): %v", err)
	}
	student, err := account.CreateStudent(t.Context(), q, account.StudentInput{
		Login:     "petrov",
		Password:  studentPassword,
		FirstName: "Пётр",
		LastName:  "Петров",
		ClassID:   classA.ID,
	})
	if err != nil {
		t.Fatalf("CreateStudent: %v", err)
	}

	return studentFixture{h: h, q: q, teacher: cookie, teacherID: me.Id, classA: classA, classB: classB, student: student}
}

func studentPath(id int64) string { return fmt.Sprintf("/api/students/%d", id) }

func classStudentsPath(classID int64) string { return fmt.Sprintf("/api/classes/%d/students", classID) }

func TestUpdateStudent(t *testing.T) {
	t.Run("rename", func(t *testing.T) {
		f := newStudentFixture(t)

		resp := doJSON(t, f.h, http.MethodPatch, studentPath(f.student.ID), api.StudentUpdate{FirstName: new("Павел")}, f.teacher)

		var got api.Student
		requireJSON(t, resp, http.StatusOK, &got)
		want := api.Student{Id: f.student.ID, ClassId: f.classA.ID, FirstName: "Павел", LastName: "Петров", Login: "petrov"}
		if got != want {
			t.Errorf("response = %+v, want %+v", got, want)
		}
	})

	t.Run("transfer to another class", func(t *testing.T) {
		f := newStudentFixture(t)

		resp := doJSON(t, f.h, http.MethodPatch, studentPath(f.student.ID), api.StudentUpdate{ClassId: &f.classB.ID}, f.teacher)

		var got api.Student
		requireJSON(t, resp, http.StatusOK, &got)
		if got.ClassId != f.classB.ID {
			t.Errorf("response class = %d, want %d", got.ClassId, f.classB.ID)
		}

		// Ученик пропал из списка старого класса и появился в новом.
		for _, tt := range []struct {
			class db.Class
			want  int
		}{
			{f.classA, 0},
			{f.classB, 1},
		} {
			var list []api.Student
			requireJSON(t, doRequest(t, f.h, http.MethodGet, classStudentsPath(tt.class.ID), f.teacher), http.StatusOK, &list)
			if len(list) != tt.want {
				t.Errorf("class %s after the transfer has %d students %+v, want %d", tt.class.Name, len(list), list, tt.want)
			}
		}
	})

	t.Run("rejected", func(t *testing.T) {
		// Ни один запрос ничего не меняет, поэтому сервер у всех случаев общий.
		f := newStudentFixture(t)

		for _, tt := range []struct {
			name       string
			id         int64
			body       any
			wantStatus int
			wantCode   api.ErrorCode
		}{
			{"empty body", f.student.ID, api.StudentUpdate{}, http.StatusBadRequest, api.ErrorCodeValidationFailed},
			// null равносилен отсутствию поля: менять по-прежнему нечего.
			{"only null fields", f.student.ID, map[string]any{"firstName": nil}, http.StatusBadRequest, api.ErrorCodeValidationFailed},
			{"blank last name", f.student.ID, api.StudentUpdate{LastName: new("  ")}, http.StatusBadRequest, api.ErrorCodeValidationFailed},
			{"unknown class", f.student.ID, api.StudentUpdate{ClassId: new(f.classB.ID + 1000)}, http.StatusBadRequest, api.ErrorCodeClassNotFound},
			{"unknown student", f.student.ID + 1000, api.StudentUpdate{FirstName: new("Павел")}, http.StatusNotFound, api.ErrorCodeNotFound},
			{"teacher", f.teacherID, api.StudentUpdate{FirstName: new("Павел")}, http.StatusNotFound, api.ErrorCodeNotFound},
		} {
			t.Run(tt.name, func(t *testing.T) {
				resp := doJSON(t, f.h, http.MethodPatch, studentPath(tt.id), tt.body, f.teacher)

				requireProblem(t, resp, tt.wantStatus, tt.wantCode)
			})
		}

		var me api.Me
		requireJSON(t, doRequest(t, f.h, http.MethodGet, "/api/session", f.teacher), http.StatusOK, &me)
		if me.FirstName != testTeacher.FirstName {
			t.Errorf("teacher's first name = %q, want %q: PATCH /api/students/{teacher id} must not touch the teacher", me.FirstName, testTeacher.FirstName)
		}
	})
}

func TestDeleteStudent(t *testing.T) {
	t.Run("deleted with its sessions", func(t *testing.T) {
		f := newStudentFixture(t)
		login := postLogin(t, f.h, "petrov", studentPassword)
		requireLoggedIn(t, f.q, login, f.student.ID)
		studentCookie := login.Cookies()[0]

		resp := doRequest(t, f.h, http.MethodDelete, studentPath(f.student.ID), f.teacher)
		if resp.StatusCode != http.StatusNoContent {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusNoContent, body)
		}

		var list []api.Student
		requireJSON(t, doRequest(t, f.h, http.MethodGet, classStudentsPath(f.classA.ID), f.teacher), http.StatusOK, &list)
		if len(list) != 0 {
			t.Errorf("class after the delete = %+v, want no students", list)
		}
		// Сессии удалены каскадом: с cookie, выданной до удаления, войти нельзя.
		requireProblem(t, doRequest(t, f.h, http.MethodGet, "/api/session", studentCookie), http.StatusUnauthorized, api.ErrorCodeUnauthorized)
		requireProblem(t, postLogin(t, f.h, "petrov", studentPassword), http.StatusUnauthorized, api.ErrorCodeInvalidCredentials)
		// Повторное удаление: ученика уже нет.
		requireProblem(t, doRequest(t, f.h, http.MethodDelete, studentPath(f.student.ID), f.teacher), http.StatusNotFound, api.ErrorCodeNotFound)
	})

	t.Run("not a student", func(t *testing.T) {
		f := newStudentFixture(t)

		for _, tt := range []struct {
			name string
			id   int64
		}{
			{"unknown id", f.student.ID + 1000},
			{"teacher", f.teacherID},
		} {
			t.Run(tt.name, func(t *testing.T) {
				resp := doRequest(t, f.h, http.MethodDelete, studentPath(tt.id), f.teacher)

				requireProblem(t, resp, http.StatusNotFound, api.ErrorCodeNotFound)
			})
		}

		// Учитель на месте и его сессия жива, ученик тоже на месте.
		var me api.Me
		requireJSON(t, doRequest(t, f.h, http.MethodGet, "/api/session", f.teacher), http.StatusOK, &me)
		var list []api.Student
		requireJSON(t, doRequest(t, f.h, http.MethodGet, classStudentsPath(f.classA.ID), f.teacher), http.StatusOK, &list)
		if len(list) != 1 {
			t.Errorf("class 7А = %+v, want the student to stay", list)
		}
	})
}
