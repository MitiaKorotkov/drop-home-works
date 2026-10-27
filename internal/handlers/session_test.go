package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/MitiaKorotkov/drop-home-works/internal/account"
	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/password"
	"github.com/MitiaKorotkov/drop-home-works/internal/ratelimit"
	"github.com/MitiaKorotkov/drop-home-works/internal/session"
	"github.com/jackc/pgx/v5"
)

func TestGetSession(t *testing.T) {
	t.Run("after login", func(t *testing.T) {
		h, _, cookie, loginMe := loginTeacher(t)

		resp := doRequest(t, h, http.MethodGet, "/api/session", cookie)

		var me api.Me
		requireJSON(t, resp, http.StatusOK, &me)
		// NOTE: DeepEqual сравнивает значения под указателями (Class), а не адреса
		if !reflect.DeepEqual(me, loginMe) {
			t.Errorf("GET /api/session = %+v, want %+v: the same user as in the login response", me, loginMe)
		}
	})

	t.Run("garbage cookie", func(t *testing.T) {
		h, _ := newTestServer(t)

		resp := doRequest(t, h, http.MethodGet, "/api/session", &http.Cookie{Name: session.CookieName, Value: "garbage"})

		requireProblem(t, resp, http.StatusUnauthorized, api.ErrorCodeUnauthorized)
	})

	t.Run("expired session", func(t *testing.T) {
		h, pool, _, me := loginTeacher(t)
		token, hash := session.NewToken()
		err := db.New(pool).CreateSession(t.Context(), db.CreateSessionParams{
			TokenHash: hash,
			UserID:    me.Id,
			ExpiresAt: time.Now().Add(-time.Minute),
		})
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		resp := doRequest(t, h, http.MethodGet, "/api/session", session.Cookie(token))

		requireProblem(t, resp, http.StatusUnauthorized, api.ErrorCodeUnauthorized)
	})

	t.Run("database unavailable", func(t *testing.T) {
		h, pool, cookie, _ := loginTeacher(t)
		pool.Close()

		resp := doRequest(t, h, http.MethodGet, "/api/session", cookie)

		// requireProblem разбирает тело как один JSON-документ: если middleware
		// допишет второй ответ после первого, разбор упадёт.
		requireProblem(t, resp, http.StatusInternalServerError, api.ErrorCodeInternalError)
	})
}

func TestLogin(t *testing.T) {
	ctx := t.Context()
	h, pool := newTestServer(t)
	q := db.New(pool)

	teacherID, err := account.CreateTeacher(ctx, q, testTeacher)
	if err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}

	class, err := q.CreateClass(ctx, "7А")
	if err != nil {
		t.Fatalf("CreateClass: %v", err)
	}

	var studentID int64
	err = pool.QueryRow(
		ctx,
		`INSERT INTO users (role, login, password_hash, last_name, first_name, class_id)
		VALUES ('student', 'student', $1, 'Петров', 'Пётр', $2)
		RETURNING id`,
		password.Hash("Student123!"),
		class.ID,
	).Scan(&studentID)
	if err != nil {
		t.Fatalf("insert student: %v", err)
	}

	t.Run("teacher login", func(t *testing.T) {
		me := requireLoggedIn(t, q, postLogin(t, h, "teacher", "Teacher123!"), teacherID)

		if me.Role != api.UserRoleTeacher {
			t.Errorf("role = %q, want %q", me.Role, api.UserRoleTeacher)
		}
		if me.FirstName != "Иван" || me.LastName != "Иванов" {
			t.Errorf("name = %q %q, want %q %q", me.FirstName, me.LastName, "Иван", "Иванов")
		}
		if me.Class != nil {
			t.Errorf("class = %+v, want nil: a teacher has no class", *me.Class)
		}
	})

	t.Run("student login", func(t *testing.T) {
		me := requireLoggedIn(t, q, postLogin(t, h, "student", "Student123!"), studentID)

		if me.Role != api.UserRoleStudent {
			t.Errorf("role = %q, want %q", me.Role, api.UserRoleStudent)
		}
		want := api.Class{Id: class.ID, Name: class.Name}
		if me.Class == nil || *me.Class != want {
			got := "nil"
			if me.Class != nil {
				got = fmt.Sprintf("%+v", *me.Class)
			}
			t.Errorf("class = %s, want %+v", got, want)
		}
	})

	t.Run("wrong password and unknown login", func(t *testing.T) {
		wrongPassword := postLogin(t, h, "teacher", "wrong-pass")
		unknownLogin := postLogin(t, h, "nobody", "Teacher123!")

		wrongPasswordBody := requireProblem(t, wrongPassword, http.StatusUnauthorized, api.ErrorCodeInvalidCredentials)
		unknownLoginBody := requireProblem(t, unknownLogin, http.StatusUnauthorized, api.ErrorCodeInvalidCredentials)

		if !bytes.Equal(wrongPasswordBody, unknownLoginBody) {
			t.Errorf("responses differ:\nwrong password: %s\nunknown login:  %s\nthe response must not reveal whether the login exists", wrongPasswordBody, unknownLoginBody)
		}
		for name, resp := range map[string]*http.Response{"wrong password": wrongPassword, "unknown login": unknownLogin} {
			if sc := resp.Header.Values("Set-Cookie"); len(sc) != 0 {
				t.Errorf("%s: Set-Cookie = %q, want none: a failed login must not start a session", name, sc)
			}
		}
	})

	t.Run("corrupted password hash", func(t *testing.T) {
		// Своя база: подтест портит данные.
		h, pool := newTestServer(t)
		if _, err := account.CreateTeacher(t.Context(), db.New(pool), testTeacher); err != nil {
			t.Fatalf("CreateTeacher: %v", err)
		}
		if _, err := pool.Exec(t.Context(), `UPDATE users SET password_hash = 'broken' WHERE login = 'teacher'`); err != nil {
			t.Fatalf("corrupt password hash: %v", err)
		}

		resp := postLogin(t, h, "teacher", "Teacher123!")

		requireProblem(t, resp, http.StatusInternalServerError, api.ErrorCodeInternalError)
		if sc := resp.Header.Values("Set-Cookie"); len(sc) != 0 {
			t.Errorf("Set-Cookie = %q, want none", sc)
		}
	})
}

func TestLogout(t *testing.T) {
	t.Run("logout", func(t *testing.T) {
		h, pool, cookie, _ := loginTeacher(t)

		requireLoggedOut(t, doRequest(t, h, http.MethodDelete, "/api/session", cookie))

		_, err := db.New(pool).GetSessionUser(t.Context(), session.HashToken(cookie.Value))
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("GetSessionUser after logout: error = %v, want pgx.ErrNoRows: the session is still in the database", err)
		}
		resp := doRequest(t, h, http.MethodGet, "/api/session", cookie)
		requireProblem(t, resp, http.StatusUnauthorized, api.ErrorCodeUnauthorized)
	})

	t.Run("other sessions survive", func(t *testing.T) {
		h, pool, phone, me := loginTeacher(t)
		resp := postLogin(t, h, testTeacher.Login, testTeacher.Password)
		requireLoggedIn(t, db.New(pool), resp, me.Id)
		laptop := resp.Cookies()[0] // requireLoggedIn уже проверил, что cookie ровно одна

		requireLoggedOut(t, doRequest(t, h, http.MethodDelete, "/api/session", phone))

		resp = doRequest(t, h, http.MethodGet, "/api/session", laptop)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET /api/session with the other session: status = %d, want %d: logout must end only the current session", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("without cookie", func(t *testing.T) {
		h, _ := newTestServer(t)

		requireLoggedOut(t, doRequest(t, h, http.MethodDelete, "/api/session", nil))
	})

	t.Run("garbage cookie", func(t *testing.T) {
		h, _ := newTestServer(t)

		requireLoggedOut(t, doRequest(t, h, http.MethodDelete, "/api/session", &http.Cookie{Name: session.CookieName, Value: "garbage"}))
	})
}

func TestLoginRateLimit(t *testing.T) {
	const ipA, ipB = "203.0.113.1:40000", "203.0.113.2:40000"
	login, right, wrong := testTeacher.Login, testTeacher.Password, "wrong-pass"

	// setup — сервер с учителем и маленькими лимитами на час: в пределах теста окно не истечёт.
	setup := func(t *testing.T, byIP, byIPLogin int) http.Handler {
		t.Helper()
		h, pool := newTestServerWithLimits(t, LoginLimits{
			ByIP:      ratelimit.New(byIP, time.Hour),
			ByIPLogin: ratelimit.New(byIPLogin, time.Hour),
		})
		if _, err := account.CreateTeacher(t.Context(), db.New(pool), testTeacher); err != nil {
			t.Fatalf("CreateTeacher: %v", err)
		}
		return h
	}
	requireStatus := func(t *testing.T, resp *http.Response, want int) {
		t.Helper()
		if resp.StatusCode != want {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, want, body)
		}
	}

	t.Run("blocked after the limit, even with the right password", func(t *testing.T) {
		h := setup(t, 100, 2)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)

		resp := postLoginFrom(t, h, ipA, login, right)

		// Иначе лимит работал бы подсказкой: 429 — пароль неверный, 200 — угадал.
		requireProblem(t, resp, http.StatusTooManyRequests, api.ErrorCodeTooManyAttempts)
		if sc := resp.Header.Values("Set-Cookie"); len(sc) != 0 {
			t.Errorf("Set-Cookie = %q, want none: a blocked attempt must not start a session", sc)
		}
	})

	t.Run("login case does not bypass the limit", func(t *testing.T) {
		h := setup(t, 100, 2)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)

		resp := postLoginFrom(t, h, ipA, "TEACHER", wrong)

		requireProblem(t, resp, http.StatusTooManyRequests, api.ErrorCodeTooManyAttempts)
	})

	t.Run("another login from the same IP is not blocked", func(t *testing.T) {
		h := setup(t, 100, 2)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)

		requireStatus(t, postLoginFrom(t, h, ipA, "nobody", wrong), http.StatusUnauthorized)
	})

	t.Run("another IP is not blocked", func(t *testing.T) {
		h := setup(t, 100, 2)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)

		// Если IP теряется по дороге, все клиенты попадают в один счётчик,
		// и чужие неудачи блокируют учителя.
		requireStatus(t, postLoginFrom(t, h, ipB, login, right), http.StatusOK)
	})

	t.Run("successful login resets the counter", func(t *testing.T) {
		h := setup(t, 100, 2)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)
		requireStatus(t, postLoginFrom(t, h, ipA, login, right), http.StatusOK)

		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)
		requireStatus(t, postLoginFrom(t, h, ipA, login, wrong), http.StatusUnauthorized)
	})

	t.Run("limit per IP across different logins", func(t *testing.T) {
		h := setup(t, 3, 100)
		for _, l := range []string{"user1", "user2", "user3"} {
			requireStatus(t, postLoginFrom(t, h, ipA, l, wrong), http.StatusUnauthorized)
		}

		requireProblem(t, postLoginFrom(t, h, ipA, "user4", wrong), http.StatusTooManyRequests, api.ErrorCodeTooManyAttempts)
		requireStatus(t, postLoginFrom(t, h, ipB, "user4", wrong), http.StatusUnauthorized)
	})

	t.Run("successful login does not reset the per-IP counter", func(t *testing.T) {
		// Иначе перебор одного пароля по многим логинам можно было бы
		// продлевать, перемежая попытки входом в свой аккаунт.
		h := setup(t, 3, 100)
		requireStatus(t, postLoginFrom(t, h, ipA, "user1", wrong), http.StatusUnauthorized)
		requireStatus(t, postLoginFrom(t, h, ipA, login, right), http.StatusOK)
		requireStatus(t, postLoginFrom(t, h, ipA, "user2", wrong), http.StatusUnauthorized)

		requireProblem(t, postLoginFrom(t, h, ipA, "user3", wrong), http.StatusTooManyRequests, api.ErrorCodeTooManyAttempts)
	})
}
