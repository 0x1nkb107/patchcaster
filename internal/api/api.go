// Package api serves the management API, version 1 (SYSREQ-035, ADR-031,
// ADR-043). The generated server in v1gen decodes and validates requests
// against openapi-v1.yaml. This package answers them, and it reports every
// error as an RFC 9457 problem.
package api

import (
	"context"
	_ "embed"
	"errors"
	"log/slog"
	"net/http"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
)

// descriptionV1 is the OpenAPI description the v1 server was generated from.
// The binary serves it, so clients can generate code from what they talk to.
//
//go:embed openapi-v1.yaml
var descriptionV1 []byte

// Service implements the v1 operations.
type Service struct {
	version string
	log     *slog.Logger
}

var _ v1gen.Handler = (*Service)(nil)

// NewService returns the v1 operations of an instance built as version.
func NewService(version string, log *slog.Logger) *Service {
	return &Service{version: version, log: log}
}

// GetInstance describes this instance.
func (s *Service) GetInstance(context.Context) (*v1gen.Instance, error) {
	return &v1gen.Instance{Version: s.version}, nil
}

// NewError turns an error a handler returned into a problem. Errors that
// already are problems keep their status. Anything else is logged and
// reported as an internal error without its message, which may reveal
// internals.
func (s *Service) NewError(ctx context.Context, err error) *v1gen.ProblemStatusCode {
	var problem *v1gen.ProblemStatusCode
	if errors.As(err, &problem) {
		return problem
	}
	s.log.ErrorContext(ctx, "management API request failed", "err", err)
	return newProblem(http.StatusInternalServerError, "")
}

// Mux serves the API and its description for the management listener.
func Mux(svc *Service) (http.Handler, error) {
	server, err := v1gen.NewServer(svc,
		v1gen.WithErrorHandler(func(ctx context.Context, w http.ResponseWriter, _ *http.Request, err error) {
			problem := problemFor(err)
			if problem.StatusCode >= 500 {
				svc.log.ErrorContext(ctx, "management API request failed", "err", err)
			}
			writeProblem(w, problem)
		}),
		v1gen.WithNotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeProblem(w, newProblem(http.StatusNotFound, ""))
		}),
		v1gen.WithMethodNotAllowed(func(w http.ResponseWriter, _ *http.Request, allowed string) {
			w.Header().Set("Allow", allowed)
			writeProblem(w, newProblem(http.StatusMethodNotAllowed, ""))
		}),
	)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(descriptionV1)
	})
	mux.Handle("/api/v1/", server)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, newProblem(http.StatusNotFound, ""))
	})
	return mux, nil
}
