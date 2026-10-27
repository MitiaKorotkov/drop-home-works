package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/classes"
)

func (s *Server) ListClasses(ctx context.Context, request api.ListClassesRequestObject) (api.ListClassesResponseObject, error) {
	rows, err := s.q.ListClasses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list classes: %w", err)
	}

	res := make([]api.Class, 0, len(rows))
	for _, row := range rows {
		res = append(res, api.Class{Id: row.ID, Name: row.Name})
	}

	return api.ListClasses200JSONResponse(res), nil
}

func (s *Server) CreateClass(ctx context.Context, request api.CreateClassRequestObject) (api.CreateClassResponseObject, error) {
	class, err := classes.CreateClass(ctx, s.q, request.Body.Name)
	if errors.Is(err, classes.ErrInvalidInput) {
		return api.CreateClass400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: api.BadRequestApplicationProblemPlusJSONResponse(
				newProblem(http.StatusBadRequest, api.ErrorCodeValidationFailed, err.Error()),
			),
		}, nil
	}
	if errors.Is(err, classes.ErrNameTaken) {
		return api.CreateClass409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: api.ConflictApplicationProblemPlusJSONResponse(
				newProblem(http.StatusConflict, api.ErrorCodeClassNameTaken, ""),
			),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return api.CreateClass201JSONResponse{
		Id:   class.ID,
		Name: class.Name,
	}, nil
}
