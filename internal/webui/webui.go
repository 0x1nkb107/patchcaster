// Package webui serves the embedded web interface on the management
// listener (ADR-032). The interface is a single-page application: it routes
// in the browser, so every path that names no file gets its fallback page.
package webui

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// fallbackPage is the page the bundle's router starts from.
const fallbackPage = "index.html"

// immutablePrefix holds the files whose names change with their content,
// which browsers may therefore keep for good.
const immutablePrefix = "_app/immutable/"

// framingPolicy is the part of the interface's Content-Security-Policy
// that a meta tag cannot carry. The bundle's page carries the rest itself,
// with the hash of its inline script (ADR-032).
const framingPolicy = "frame-ancestors 'none'"

// notBuiltPolicy allows the page of a binary without the bundle nothing,
// because it has no scripts, styles or images.
const notBuiltPolicy = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// notBuilt is what a binary built without the bundle serves instead.
const notBuilt = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>PatchCaster</title></head>
<body>
<h1>PatchCaster</h1>
<p>This binary was built without its web interface. Build it with <code>just web-build</code> before the binary, or use the management API at <code>/api/v1/</code>.</p>
</body>
</html>
`

// Handler serves the bundle in bundle: its files as they are, and the
// fallback page for every other path. Without a fallback page, the bundle
// was not built, and every path gets a page that says so. Either way, it
// serves only GET and HEAD, under a Content-Security-Policy.
func Handler(bundle fs.FS) http.Handler {
	if _, err := fs.Stat(bundle, fallbackPage); err != nil {
		return onlyReads(withPolicy(notBuiltPolicy, http.HandlerFunc(serveNotBuilt)))
	}
	return onlyReads(withPolicy(framingPolicy, &handler{bundle: bundle}))
}

// withPolicy sends policy as the Content-Security-Policy of every answer.
func withPolicy(policy string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", policy)
		next.ServeHTTP(w, r)
	})
}

// onlyReads refuses every method but GET and HEAD, because the interface
// only serves pages.
func onlyReads(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type handler struct {
	bundle fs.FS
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if !h.isFile(name) {
		h.serveFallback(w, r)
		return
	}
	if strings.HasPrefix(name, immutablePrefix) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, r, h.bundle, name)
}

// isFile reports whether name is a file of the bundle, not a directory.
func (h *handler) isFile(name string) bool {
	if name == "" || name == fallbackPage {
		return false
	}
	info, err := fs.Stat(h.bundle, name)
	return err == nil && !info.IsDir()
}

// serveFallback serves the fallback page, which browsers must check again
// on every visit, because it names the current release's files.
func (h *handler) serveFallback(w http.ResponseWriter, r *http.Request) {
	page, err := fs.ReadFile(h.bundle, fallbackPage)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, fs.ErrNotExist) {
			status = http.StatusNotFound
		}
		http.Error(w, http.StatusText(status), status)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(page)
}

func serveNotBuilt(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(notBuilt))
}
