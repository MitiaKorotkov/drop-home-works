package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
)

var _ api.StrictServerInterface = (*Server)(nil)

type Server struct {
	api.StrictServerInterface

	logger *slog.Logger
}

func NewServer(logger *slog.Logger) *Server {
	return &Server{logger: logger}
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

func writeProblem(w http.ResponseWriter, status int, code api.ErrorCode, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	json.NewEncoder(w).Encode(newProblem(status, code, detail))
}

func Register(mux *http.ServeMux, s *Server) {
	requestError := func(w http.ResponseWriter, r *http.Request, err error) {
		writeProblem(w, http.StatusBadRequest, api.ErrorCodeValidationFailed, err.Error())
	}
	responseError := func(w http.ResponseWriter, r *http.Request, err error) {
		s.logger.Error("response error", "method", r.Method, "path", r.URL.Path, "error", err)
		writeProblem(w, http.StatusInternalServerError, api.ErrorCodeInternalError, "")
	}

	si := api.NewStrictHandlerWithOptions(s, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestError,
		ResponseErrorHandlerFunc: responseError,
	})

	api.HandlerWithOptions(si, api.StdHTTPServerOptions{
		BaseURL:          "/api",
		BaseRouter:       mux,
		ErrorHandlerFunc: requestError,
	})
}

func (s *Server) GetSession(ctx context.Context, request api.GetSessionRequestObject) (api.GetSessionResponseObject, error) {
	return api.GetSession401ApplicationProblemPlusJSONResponse{
		UnauthorizedApplicationProblemPlusJSONResponse: api.UnauthorizedApplicationProblemPlusJSONResponse(
			newProblem(http.StatusUnauthorized, api.ErrorCodeUnauthorized, ""),
		),
	}, nil
}
