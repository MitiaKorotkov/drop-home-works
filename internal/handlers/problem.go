package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
)

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

func requestError(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, http.StatusBadRequest, api.ErrorCodeValidationFailed, err.Error())
}

func (s *Server) responseError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("response error", "method", r.Method, "path", r.URL.Path, "error", err)
	writeProblem(w, http.StatusInternalServerError, api.ErrorCodeInternalError, "")
}
