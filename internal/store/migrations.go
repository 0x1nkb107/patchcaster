package store

import "embed"

// MigrationsFS carries the goose SQL files inside the binary so both
// "patchcaster migrate" and serve-time migration use the exact schema the
// binary was compiled with (SYSREQ-021).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
