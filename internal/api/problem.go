package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
)

// problemType is the type of every problem the API reports so far: RFC 9457
// says "about:blank" when the status code tells the whole story.
const problemType = "about:blank"

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

// problemFor maps an error that the generated server met before or after a
// handler ran, such as a request it could not decode, to a problem.
func problemFor(err error) *v1gen.ProblemStatusCode {
	var coded interface{ Code() int }
	if errors.As(err, &coded) && coded.Code() >= 400 && coded.Code() < 500 {
		return newProblem(coded.Code(), requestProblemDetail(err))
	}
	return newProblem(http.StatusInternalServerError, "")
}

// requestProblemDetail explains a rejected request without echoing its
// content, because the body may hold a secret.
func requestProblemDetail(err error) string {
	var decodeBody *ogenerrors.DecodeBodyError
	if errors.As(err, &decodeBody) {
		return "the request body does not match the API description"
	}
	var decodeParams *ogenerrors.DecodeParamsError
	if errors.As(err, &decodeParams) {
		return "a request parameter does not match the API description"
	}
	return "the request does not match the API description"
}
