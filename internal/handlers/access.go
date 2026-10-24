package handlers

import (
	"context"
	"net/http"

	"github.com/MitiaKorotkov/drop-home-works/internal/api"
	"github.com/MitiaKorotkov/drop-home-works/internal/db"
)

type access int

const (
	public access = iota
	anyUser
	teacherOnly
	studentOnly
)

var operationAccess = map[string]access{
	"Login":                    public,
	"Logout":                   public,
	"GetSession":               anyUser,
	"CreateAssignment":         teacherOnly,
	"CreateClass":              teacherOnly,
	"CreateStudent":            teacherOnly,
	"DeleteAssignment":         teacherOnly,
	"DeleteAssignmentFile":     teacherOnly,
	"DeleteStudent":            teacherOnly,
	"DownloadAssignmentFile":   teacherOnly,
	"DownloadSubmission":       teacherOnly,
	"GetAssignment":            teacherOnly,
	"ListAssignments":          teacherOnly,
	"ListClasses":              teacherOnly,
	"ListClassStudents":        teacherOnly,
	"ListClassSubmissions":     teacherOnly,
	"SetClassDeadline":         teacherOnly,
	"SetStudentPassword":       teacherOnly,
	"SetSubmissionScore":       teacherOnly,
	"SetSubmissionStatus":      teacherOnly,
	"UpdateAssignment":         teacherOnly,
	"UpdateStudent":            teacherOnly,
	"UploadAssignmentFile":     teacherOnly,
	"DownloadMyAssignmentFile": studentOnly,
	"DownloadMySubmission":     studentOnly,
	"GetMyAssignment":          studentOnly,
	"ListMyAssignments":        studentOnly,
	"UploadMySubmission":       studentOnly,
}

func checkRole(role db.UserRole, a access) bool {
	switch a {
	case public, anyUser:
		return true
	case teacherOnly:
		return role == db.UserRoleTeacher
	case studentOnly:
		return role == db.UserRoleStudent
	}

	return false
}

func (s *Server) requireAccess(f api.StrictHandlerFunc, operationID string) api.StrictHandlerFunc {
	a, exists := operationAccess[operationID]

	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		if !exists {
			s.logger.Error("operation has no access rule", "operation", operationID)
			writeProblem(w, http.StatusInternalServerError, api.ErrorCodeInternalError, "")
			return nil, nil
		}

		if a == public {
			return f(ctx, w, r, request)
		}

		cs, ok := sessionFrom(ctx)
		if !ok {
			writeProblem(w, http.StatusUnauthorized, api.ErrorCodeUnauthorized, "")
			return nil, nil
		}
		if !checkRole(cs.user.Role, a) {
			writeProblem(w, http.StatusForbidden, api.ErrorCodeForbidden, "")
			return nil, nil
		}

		return f(ctx, w, r, request)
	}
}
