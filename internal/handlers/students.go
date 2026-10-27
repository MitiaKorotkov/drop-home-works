package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/MitiaKorotkov/drop-home-works/internal/account"
	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
)

func newStudent(row db.ListClassStudentsRow) api.Student {
	return api.Student{
		ClassId:   row.ClassID,
		FirstName: row.FirstName,
		Id:        row.ID,
		LastName:  row.LastName,
		Login:     row.Login,
	}
}

func (s *Server) CreateStudent(ctx context.Context, request api.CreateStudentRequestObject) (api.CreateStudentResponseObject, error) {
	row, err := account.CreateStudent(ctx, s.q, account.StudentInput{
		Login:     request.Body.Login,
		Password:  request.Body.Password,
		FirstName: request.Body.FirstName,
		LastName:  request.Body.LastName,
		ClassID:   request.Body.ClassId,
	})
	switch {
	case errors.Is(err, account.ErrInvalidInput):
		return api.CreateStudent400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest(api.ErrorCodeValidationFailed, err.Error()),
		}, nil
	case errors.Is(err, account.ErrClassNotFound):
		return api.CreateStudent400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest(api.ErrorCodeClassNotFound, ""),
		}, nil
	case errors.Is(err, account.ErrLoginTaken):
		return api.CreateStudent409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: conflict(api.ErrorCodeLoginTaken),
		}, nil
	case err != nil:
		return nil, err
	}

	return api.CreateStudent201JSONResponse(newStudent(db.ListClassStudentsRow(row))), nil
}

func (s *Server) ListClassStudents(ctx context.Context, request api.ListClassStudentsRequestObject) (api.ListClassStudentsResponseObject, error) {
	exists, err := s.q.ClassExists(ctx, request.ClassId)
	if err != nil {
		return nil, fmt.Errorf("class exists: %w", err)
	}
	if !exists {
		return api.ListClassStudents404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(api.ErrorCodeNotFound, ""),
		}, nil
	}

	rows, err := s.q.ListClassStudents(ctx, request.ClassId)
	if err != nil {
		return nil, fmt.Errorf("list class students: %w", err)
	}

	res := make([]api.Student, 0, len(rows))
	for _, row := range rows {
		res = append(res, newStudent(row))
	}

	return api.ListClassStudents200JSONResponse(res), nil
}
