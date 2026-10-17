// Package web embeds the bundled web interface into the binary (ADR-032).
// The bundle lives in dist/app once `just web-build` has run. dist/.gitkeep
// keeps the pattern valid before that, so Go code builds without Node.
package web

import "embed"

// Dist holds dist/, with the bundle under dist/app when it was built.
//
//go:embed all:dist
var Dist embed.FS
