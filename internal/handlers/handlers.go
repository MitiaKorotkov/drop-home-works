package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
	"github.com/MitiaKorotkov/drop-home-works/internal/ratelimit"
)

type LoginLimits struct {
	ByIP      *ratelimit.Limiter
	ByIPLogin *ratelimit.Limiter
}

func DefaultLoginLimits() LoginLimits {
	return LoginLimits{
		ByIP:      ratelimit.New(100, 15*time.Minute),
		ByIPLogin: ratelimit.New(5, 15*time.Minute),
	}
}

var _ api.StrictServerInterface = (*Server)(nil)

type Server struct {
	api.StrictServerInterface

	logger         *slog.Logger
	q              *db.Queries
	loginByIP      *ratelimit.Limiter
	loginByIPLogin *ratelimit.Limiter
}

func NewServer(logger *slog.Logger, q *db.Queries, limits LoginLimits) *Server {
	return &Server{logger: logger, q: q, loginByIP: limits.ByIP, loginByIPLogin: limits.ByIPLogin}
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
		Middlewares:      []api.MiddlewareFunc{s.loadSession, loadClientIP},
		ErrorHandlerFunc: requestError,
	})
}
