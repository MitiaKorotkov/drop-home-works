package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/MitiaKorotkov/drop-home-works/internal/account"
	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/session"
)

var _ api.StrictServerInterface = (*Server)(nil)

type Server struct {
	api.StrictServerInterface

	logger *slog.Logger
	q      *db.Queries
}

type sessionKey struct{}

type currentSession struct {
	user      account.User
	tokenHash []byte
}

func newProblem(status int, code api.ErrorCode, detail string) api.Problem {
	p := api.Problem{
		Code:   code,
		Status: int32(status),
		Title:  http.StatusText(status),
	}
	if detail != "" {
		p.Detail = &detail
	}

	return p
}

func newMe(user account.User) api.Me {
	me := api.Me{
		FirstName: user.FirstName,
		Id:        user.ID,
		LastName:  user.LastName,
		Role:      api.UserRole(user.Role),
	}
	if user.ClassID != nil {
		me.Class = &api.Class{
			Id:   *user.ClassID,
			Name: *user.ClassName,
		}
	}

	return me
}

func writeProblem(w http.ResponseWriter, status int, code api.ErrorCode, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	json.NewEncoder(w).Encode(newProblem(status, code, detail))
}

func withSession(ctx context.Context, session currentSession) context.Context {
	return context.WithValue(ctx, sessionKey{}, session)
}

func sessionFrom(ctx context.Context) (currentSession, bool) {
	s, ok := ctx.Value(sessionKey{}).(currentSession)
	return s, ok
}

func NewServer(logger *slog.Logger, q *db.Queries) *Server {
	return &Server{logger: logger, q: q}
}

func Register(mux *http.ServeMux, s *Server) {
	requestError := func(w http.ResponseWriter, r *http.Request, err error) {
		writeProblem(w, http.StatusBadRequest, api.ErrorCodeValidationFailed, err.Error())
	}
	responseError := func(w http.ResponseWriter, r *http.Request, err error) {
		s.logger.Error("response error", "method", r.Method, "path", r.URL.Path, "error", err)
		writeProblem(w, http.StatusInternalServerError, api.ErrorCodeInternalError, "")
	}

	loadSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(session.CookieName)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			hash := session.HashToken(cookie.Value)
			u, err := account.UserBySession(r.Context(), s.q, hash)
			if errors.Is(err, account.ErrNoSession) {
				next.ServeHTTP(w, r)
				return
			}
			if err != nil {
				s.logger.Error("load session", "error", err)
				writeProblem(w, http.StatusInternalServerError, api.ErrorCodeInternalError, "")
				return
			}

			ctx := withSession(r.Context(), currentSession{user: u, tokenHash: hash})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	si := api.NewStrictHandlerWithOptions(s, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestError,
		ResponseErrorHandlerFunc: responseError,
	})

	api.HandlerWithOptions(si, api.StdHTTPServerOptions{
		BaseURL:          "/api",
		BaseRouter:       mux,
		Middlewares:      []api.MiddlewareFunc{loadSession},
		ErrorHandlerFunc: requestError,
	})
}

func (s *Server) GetSession(ctx context.Context, request api.GetSessionRequestObject) (api.GetSessionResponseObject, error) {
	if cs, ok := sessionFrom(ctx); ok {
		return api.GetSession200JSONResponse(newMe(cs.user)), nil
	}

	return api.GetSession401ApplicationProblemPlusJSONResponse{
		UnauthorizedApplicationProblemPlusJSONResponse: api.UnauthorizedApplicationProblemPlusJSONResponse(
			newProblem(http.StatusUnauthorized, api.ErrorCodeUnauthorized, ""),
		),
	}, nil
}

func (s *Server) Login(ctx context.Context, request api.LoginRequestObject) (api.LoginResponseObject, error) {
	user, err := account.Authenticate(ctx, s.q, request.Body.Login, request.Body.Password)
	if errors.Is(err, account.ErrInvalidCredentials) {
		return api.Login401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: api.UnauthorizedApplicationProblemPlusJSONResponse(
				newProblem(http.StatusUnauthorized, api.ErrorCodeInvalidCredentials, ""),
			),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	token, hash := session.NewToken()
	err = s.q.CreateSession(ctx, db.CreateSessionParams{
		TokenHash: hash,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(session.Lifetime),
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	cookie := session.Cookie(token).String()

	return api.Login200JSONResponse{
		Body:    newMe(user),
		Headers: api.Login200ResponseHeaders{SetCookie: &cookie},
	}, nil
}

func (s *Server) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	if cs, ok := sessionFrom(ctx); ok {
		if err := s.q.DeleteSession(ctx, cs.tokenHash); err != nil {
			return nil, fmt.Errorf("delete session: %w", err)
		}
	}

	cleared := session.ClearCookie().String()
	return api.Logout204Response{
		Headers: api.Logout204ResponseHeaders{SetCookie: &cleared},
	}, nil
}
