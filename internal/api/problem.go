package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/ogen-go/ogen/validate"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
)

// problemType is the type of every problem the API reports so far: RFC 9457
// says "about:blank" when the status code tells the whole story.
const problemType = "about:blank"

// ifMatchHeader is the header every change must send (ADR-043).
const ifMatchHeader = "If-Match"

// newProblem builds an RFC 9457 problem for status, titled with the status
// text (ADR-043).
func newProblem(status int, detail string, fields ...v1gen.FieldError) *v1gen.ProblemStatusCode {
	problem := v1gen.Problem{
		Type:   problemType,
		Title:  http.StatusText(status),
		Status: status,
		Errors: fields,
	}
	if detail != "" {
		problem.Detail = v1gen.NewOptString(detail)
	}
	return &v1gen.ProblemStatusCode{StatusCode: status, Response: problem}
}

// fieldProblem builds a problem about one request field.
func fieldProblem(status int, detail, path, message string) *v1gen.ProblemStatusCode {
	return newProblem(status, detail, v1gen.FieldError{
		Path:     path,
		Messages: []string{message},
	})
}

// writeProblem writes a problem outside the generated handlers, such as for
// an unknown path.
func writeProblem(w http.ResponseWriter, p *v1gen.ProblemStatusCode) {
	body, err := p.Response.MarshalJSON()
	if err != nil {
		body, _ = json.Marshal(map[string]any{"type": problemType, "title": "Internal Server Error", "status": 500})
		p.StatusCode = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.StatusCode)
	_, _ = w.Write(body)
}

// ProblemHandler answers every request with a problem of status, for
// refusals that happen before a request reaches the API, so that they read
// like the API's own.
func ProblemHandler(status int, detail string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, newProblem(status, detail))
	})
}

// problemFor maps any error of a request to its problem. A handler's problem
// stays as it is. Errors the generated server met before the handler ran
// become client problems that name the fields at fault. Anything else is an
// internal error.
func problemFor(err error) *v1gen.ProblemStatusCode {
	var problem *v1gen.ProblemStatusCode
	if errors.As(err, &problem) {
		return problem
	}
	var unauthenticated *ogenerrors.SecurityError
	if errors.As(err, &unauthenticated) {
		return newProblem(http.StatusUnauthorized, "log in first")
	}
	var param *ogenerrors.DecodeParamError
	if errors.As(err, &param) {
		return paramProblem(param)
	}
	var request *ogenerrors.DecodeRequestError
	if errors.As(err, &request) {
		return requestProblem(request)
	}
	return newProblem(http.StatusInternalServerError, "")
}

// paramProblem reports a request parameter that does not match the API
// description. A change without If-Match has its own status (ADR-043).
func paramProblem(param *ogenerrors.DecodeParamError) *v1gen.ProblemStatusCode {
	missing := isMissing(param.Err)
	if missing && param.Name == ifMatchHeader {
		return newProblem(http.StatusPreconditionRequired,
			"send the resource's ETag in If-Match, so that no change is lost")
	}
	message := param.Err.Error()
	if missing {
		message = "is required"
	}
	return fieldProblem(http.StatusBadRequest,
		"a request parameter does not match the API description", param.Name, message)
}

// isMissing reports whether a parameter error says that the parameter is
// absent. The generated decoders report that as a validation error that
// lists the parameter as a missing field.
func isMissing(err error) bool {
	var invalid *validate.Error
	if !errors.As(err, &invalid) {
		return errors.Is(err, validate.ErrFieldRequired)
	}
	for _, field := range invalid.Fields {
		if errors.Is(field.Error, validate.ErrFieldRequired) {
			return true
		}
	}
	return false
}

// requestProblem reports a request body that does not match the API
// description. It never echoes the body, because the body may hold a
// secret.
func requestProblem(request *ogenerrors.DecodeRequestError) *v1gen.ProblemStatusCode {
	var contentType *validate.InvalidContentTypeError
	if errors.As(request, &contentType) {
		return newProblem(http.StatusUnsupportedMediaType, "send the request body as application/json")
	}
	var invalid *validate.Error
	if errors.As(request, &invalid) {
		return newProblem(http.StatusBadRequest,
			"the request body does not match the API description", fieldErrors("", invalid)...)
	}
	return newProblem(http.StatusBadRequest, "the request body is not JSON of the shape the API describes")
}

// fieldErrors lists the fields a validation error names, each by its path the
// way form libraries spell it, such as "routes[0].sink_id" (ADR-043).
func fieldErrors(prefix string, invalid *validate.Error) []v1gen.FieldError {
	var fields []v1gen.FieldError
	for _, field := range invalid.Fields {
		path := joinPath(prefix, field.Name)
		var nested *validate.Error
		if errors.As(field.Error, &nested) {
			fields = append(fields, fieldErrors(path, nested)...)
			continue
		}
		fields = append(fields, v1gen.FieldError{
			Path:     path,
			Messages: []string{fieldMessage(field.Error)},
		})
	}
	return fields
}

// joinPath appends a field name to a path. Array indexes, which the
// generated validators name "[0]", attach without a dot.
func joinPath(prefix, name string) string {
	if prefix == "" || strings.HasPrefix(name, "[") {
		return prefix + name
	}
	return prefix + "." + name
}

// fieldMessage words a field's validation error. The generated validators
// word every error but a missing field themselves.
func fieldMessage(err error) string {
	if errors.Is(err, validate.ErrFieldRequired) {
		return "is required"
	}
	return err.Error()
}
