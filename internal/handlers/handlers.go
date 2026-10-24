package handlers

import (
	"log/slog"
	"net/http"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
)

var _ api.StrictServerInterface = (*Server)(nil)

type Server struct {
	api.StrictServerInterface

	logger *slog.Logger
	q      *db.Queries
}

func NewServer(logger *slog.Logger, q *db.Queries) *Server {
	return &Server{logger: logger, q: q}
}

func Register(mux *http.ServeMux, s *Server) {
	si := api.NewStrictHandlerWithOptions(
		s,
		[]api.StrictMiddlewareFunc{
			s.requireAccess,
		},
		api.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  requestError,
			ResponseErrorHandlerFunc: s.responseError,
		},
	)

	api.HandlerWithOptions(si, api.StdHTTPServerOptions{
		BaseURL:          "/api",
		BaseRouter:       mux,
		Middlewares:      []api.MiddlewareFunc{s.loadSession},
		ErrorHandlerFunc: requestError,
	})
}
