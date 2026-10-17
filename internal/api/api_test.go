package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMux(t *testing.T) http.Handler {
	t.Helper()
	mux, err := Mux(NewService("1.2.3", slog.New(slog.DiscardHandler)))
	require.NoError(t, err)
	return mux
}

// problemBody is the RFC 9457 shape the tests read back.
type problemBody struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func serve(t *testing.T, mux http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func readProblem(t *testing.T, rec *httptest.ResponseRecorder) problemBody {
	t.Helper()
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	var p problemBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	return p
}

func TestGetInstanceReportsTheVersion(t *testing.T) {
	// act
	rec := serve(t, newTestMux(t), http.MethodGet, "/api/v1/instance")

	// assert
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"version":"1.2.3"}`, rec.Body.String())
}

func TestTheDescriptionIsServed(t *testing.T) {
	// act
	rec := serve(t, newTestMux(t), http.MethodGet, "/api/v1/openapi.yaml")

	// assert
	require.Equal(t, http.StatusOK, rec.Code)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	assert.Equal(t, descriptionV1, body)
}

func TestErrorsAreProblems(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		status int
	}{
		{"an unknown API path", http.MethodGet, "/api/v1/nothing-here", http.StatusNotFound},
		{"a path outside the API", http.MethodGet, "/elsewhere", http.StatusNotFound},
		{"a method the operation does not take", http.MethodDelete, "/api/v1/instance", http.StatusMethodNotAllowed},
	}
	mux := newTestMux(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			rec := serve(t, mux, tc.method, tc.path)

			// assert
			require.Equal(t, tc.status, rec.Code)
			p := readProblem(t, rec)
			assert.Equal(t, problemType, p.Type)
			assert.Equal(t, tc.status, p.Status)
			assert.Equal(t, http.StatusText(tc.status), p.Title)
		})
	}
}

func TestAMethodNotAllowedNamesTheAllowedOnes(t *testing.T) {
	// act
	rec := serve(t, newTestMux(t), http.MethodPost, "/api/v1/instance")

	// assert
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET", rec.Header().Get("Allow"))
}

func TestAnUnexpectedErrorRevealsNothing(t *testing.T) {
	// arrange
	svc := NewService("1.2.3", slog.New(slog.DiscardHandler))

	// act
	problem := svc.NewError(context.Background(), errors.New("password=hunter2 in a driver error"))

	// assert
	assert.Equal(t, http.StatusInternalServerError, problem.StatusCode)
	assert.False(t, problem.Response.Detail.IsSet(), "an internal error must not echo its cause")
}

func TestAProblemReturnedByAHandlerKeepsItsStatus(t *testing.T) {
	// arrange
	svc := NewService("1.2.3", slog.New(slog.DiscardHandler))
	conflict := newProblem(http.StatusConflict, "the resource changed meanwhile")

	// act
	problem := svc.NewError(context.Background(), conflict)

	// assert
	assert.Same(t, conflict, problem)
}
