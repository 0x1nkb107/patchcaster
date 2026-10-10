// Package routing holds the project-path to Mattermost-channel routing table
// (SYSREQ-009). Resolution is exact and case-sensitive; there is no default or
// catch-all channel — an unmapped path never falls back to one.
package routing

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	z "github.com/Oudwins/zog"
	"go.yaml.in/yaml/v4" // ADR-013
)

// Table maps GitLab project paths to Mattermost channel identifiers.
type Table struct {
	byProject map[string]string
}

// Entry is one routing row: project path -> channel identifier.
type Entry struct {
	Project string `yaml:"project"`
	Channel string `yaml:"channel"`
}

type file struct {
	Routes []Entry `yaml:"routes"`
}

// Load reads and validates the routing document. An invalid table must prevent
// serving (SYSREQ-022 posture): errors name the offending row.
func Load(path string) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open routing file %q: %w", path, err)
	}
	return Parse(data)
}

// Parse validates routing content (exported for tests and startup injection).
func Parse(data []byte) (*Table, error) {
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse routing table: %w", err)
	}
	if err := validateTable(&f); err != nil {
		return nil, fmt.Errorf("invalid routing table: %w", err)
	}
	t := &Table{byProject: make(map[string]string, len(f.Routes))}
	for _, e := range f.Routes {
		t.byProject[e.Project] = e.Channel
	}
	return t, nil
}

// tableSchema asserts the per-row invariants of the decoded document (ADR-012):
// every row must carry a project path and a channel. Cross-row rules live in
// uniqueProjects so that all violations land in one aggregated report.
var tableSchema = z.Struct(z.Shape{
	"Routes": z.Slice(z.Struct(z.Shape{
		"Project": z.String().Required(z.Message("project path must not be empty")),
		"Channel": z.String().Required(z.Message("channel identifier must not be empty")),
	})),
})

// uniqueProjectsSchema rejects duplicate project paths: the later row would
// silently shadow the earlier one (SYSREQ-009 wants one owner per path).
// v0.23 has no struct-level tests (ADR-012), so the cross-row rule is a
// whole-document custom schema.
var uniqueProjectsSchema = z.CustomFunc(func(f *file, ctx z.Ctx) bool {
	seen := make(map[string]bool, len(f.Routes))
	for i, e := range f.Routes {
		if e.Project == "" {
			continue // emptiness is tableSchema's report, not a duplicate
		}
		if seen[e.Project] {
			ctx.AddIssue(ctx.Issue().
				SetPath([]string{"Routes", strconv.Itoa(i)}).
				SetMessage(fmt.Sprintf("duplicate project %q", e.Project)))
		}
		seen[e.Project] = true
	}
	// Always true: the manual issues are the report; returning false would
	// add a redundant generic auto-issue on top of them.
	return true
})

// validateTable reports every row violation in one error, each naming its row
// index (SYSREQ-022 posture): a broken table must not serve, and the operator
// should not have to fix one row per deploy to find the next.
func validateTable(f *file) error {
	issues := append(tableSchema.Validate(f), uniqueProjectsSchema.Validate(f)...)
	if len(issues) == 0 {
		return nil
	}
	errs := make([]error, 0, len(issues))
	for _, issue := range issues {
		errs = append(errs, fmt.Errorf("%s: %s", location(issue.Path), issue.Message))
	}
	return errors.Join(errs...)
}

// location renders a schema path (e.g. Routes/[1]/Project) as an operator-facing
// row reference.
func location(path []string) string {
	if len(path) >= 2 && path[0] == "Routes" {
		return "routing entry " + strings.Trim(path[1], "[]")
	}
	return "routing table"
}

// Channel resolves a project path to its destination channel identifier.
// Many projects may name the same channel (SYSREQ-011); the lookup never
// invents one.
func (t *Table) Channel(projectPath string) (string, bool) {
	ch, ok := t.byProject[projectPath]
	return ch, ok
}

// ErrNoRoutes is returned for an empty routes list; an empty table forwards
// nothing and is almost certainly operator error.
var ErrNoRoutes = errors.New("routing table has no routes")
