// Package routing holds the project-path to Mattermost-channel routing table
// (SYSREQ-009). Resolution is exact and case-sensitive; there is no default or
// catch-all channel — an unmapped path never falls back to one.
package routing

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
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
	t := &Table{byProject: make(map[string]string, len(f.Routes))}
	for i, e := range f.Routes {
		if e.Project == "" {
			return nil, fmt.Errorf("routing entry %d: empty project", i)
		}
		if e.Channel == "" {
			return nil, fmt.Errorf("routing entry %d (project %q): empty channel", i, e.Project)
		}
		if _, dup := t.byProject[e.Project]; dup {
			return nil, fmt.Errorf("routing entry %d: duplicate project %q", i, e.Project)
		}
		t.byProject[e.Project] = e.Channel
	}
	return t, nil
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
