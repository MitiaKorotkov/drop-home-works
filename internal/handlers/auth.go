package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/MitiaKorotkov/drop-home-works/internal/account"
	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/session"
)

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
