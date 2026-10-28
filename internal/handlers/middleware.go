package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"

	"github.com/MitiaKorotkov/drop-home-works/internal/account"
	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/session"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
)

type sessionKey struct{}

type currentSession struct {
	user      account.User
	tokenHash []byte
}

func withSession(ctx context.Context, cs currentSession) context.Context {
	return context.WithValue(ctx, sessionKey{}, cs)
}

func sessionFrom(ctx context.Context) (currentSession, bool) {
	s, ok := ctx.Value(sessionKey{}).(currentSession)
	return s, ok
}

func (s *Server) loadSession(next http.Handler) http.Handler {
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

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}

	addr = addr.Unmap()
	if addr.Is6() {
		return netip.PrefixFrom(addr, 64).Masked().String()
	}
	return addr.String()
}

type clientIPKey struct{}

func clientIPFrom(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

func loadClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), clientIPKey{}, clientIP(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) validateRequest(router routers.Router) api.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, pathParams, err := router.FindRoute(r)
			if err != nil {
				s.logger.Error("find route", "error", err, "method", r.Method, "path", r.URL.Path)
				writeProblem(w, http.StatusInternalServerError, api.ErrorCodeInternalError, "")
				return
			}

			opts := &openapi3filter.Options{
				AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
				ExcludeRequestBody: route.Operation.OperationID == "UploadMySubmission" || route.Operation.OperationID == "UploadAssignmentFile",
			}
			opts.WithCustomSchemaErrorFunc(func(err *openapi3.SchemaError) string { return err.Reason })

			err = openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
				Request:    r,
				PathParams: pathParams,
				Route:      route,
				Options:    opts,
			})
			if err != nil {
				writeProblem(w, http.StatusBadRequest, api.ErrorCodeValidationFailed, err.Error())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
