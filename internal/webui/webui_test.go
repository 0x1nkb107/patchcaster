package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bundle is a built interface as the adapter writes it.
var bundle = fstest.MapFS{
	"index.html":                  {Data: []byte("<!doctype html><title>app</title>")},
	"favicon.svg":                 {Data: []byte("<svg/>")},
	"_app/immutable/entry/app.js": {Data: []byte("console.log(1)")},
	"_app/version.json":           {Data: []byte(`{"version":"1"}`)},
}

func get(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestTheBundleIsServed(t *testing.T) {
	h := Handler(bundle)
	tests := []struct {
		name      string
		target    string
		wantBody  string
		wantCache string
	}{
		{"the root", "/", "<!doctype html><title>app</title>", "no-cache"},
		{"a page the router knows", "/tokens", "<!doctype html><title>app</title>", "no-cache"},
		{"a nested page", "/teams/3/routes", "<!doctype html><title>app</title>", "no-cache"},
		{"a hashed file", "/_app/immutable/entry/app.js", "console.log(1)", "public, max-age=31536000, immutable"},
		{"a plain file", "/favicon.svg", "<svg/>", "no-cache"},
		{"a directory", "/_app/", "<!doctype html><title>app</title>", "no-cache"},
		{"a path climbing out", "/../../etc/passwd", "<!doctype html><title>app</title>", "no-cache"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			rec := get(t, h, http.MethodGet, tc.target)

			// assert
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tc.wantBody, rec.Body.String())
			assert.Equal(t, tc.wantCache, rec.Header().Get("Cache-Control"))
		})
	}
}

func TestTheFallbackPageIsHTML(t *testing.T) {
	// act
	rec := get(t, Handler(bundle), http.MethodGet, "/setup")

	// assert
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
}

func TestOnlyReadsAreServed(t *testing.T) {
	tests := []struct {
		name   string
		bundle fstest.MapFS
	}{
		{"with the bundle", bundle},
		{"without the bundle", fstest.MapFS{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			rec := get(t, Handler(tc.bundle), http.MethodPost, "/")

			// assert
			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
			assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
		})
	}
}

func TestEveryAnswerForbidsFraming(t *testing.T) {
	tests := []struct {
		name   string
		bundle fstest.MapFS
		target string
		want   string
	}{
		{"the fallback page", bundle, "/tokens", "frame-ancestors 'none'"},
		{"a file of the bundle", bundle, "/favicon.svg", "frame-ancestors 'none'"},
		{"the page without the bundle", fstest.MapFS{}, "/", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			rec := get(t, Handler(tc.bundle), http.MethodGet, tc.target)

			// assert
			assert.Equal(t, tc.want, rec.Header().Get("Content-Security-Policy"))
		})
	}
}

func TestABinaryWithoutTheBundleSaysSo(t *testing.T) {
	// act
	rec := get(t, Handler(fstest.MapFS{}), http.MethodGet, "/tokens")

	// assert
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "built without its web interface")
}
