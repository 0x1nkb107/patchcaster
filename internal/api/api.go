// Package api serves the management API, version 1 (SYSREQ-035, ADR-031,
// ADR-043). The generated server in v1gen decodes and validates requests
// against openapi-v1.yaml. This package answers them, and it reports every
// error as an RFC 9457 problem.
package api

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
)

// descriptionV1 is the OpenAPI description the v1 server was generated from.
// The binary serves it, so clients can generate code from what they talk to.
//
//go:embed openapi-v1.yaml
var descriptionV1 []byte

// Routes adds the API and its description to mux, under /api/. Any other
// path under /api/ answers with a 404 problem.
func Routes(mux *http.ServeMux, svc *Service) error {
	server, err := v1gen.NewServer(svc, security{svc},
		v1gen.WithErrorHandler(func(ctx context.Context, w http.ResponseWriter, _ *http.Request, err error) {
			writeProblem(w, svc.NewError(ctx, err))
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
		return fmt.Errorf("build v1 server: %w", err)
	}
	mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(descriptionV1)
	})
	mux.Handle("/api/", server)
	return nil
}
