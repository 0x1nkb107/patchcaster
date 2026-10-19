package message

import (
	"embed"
	"fmt"
)

// The built-in templates ship with the binary (SYSREQ-025), one per kind of
// message, and read Version1.
//
//go:embed templates/*.tmpl
var builtinFS embed.FS

// Builtins parses the built-in templates by the kind of message they write.
// A failure is a bug in the binary, so callers treat it as fatal.
func Builtins() (map[Kind]*Template, error) {
	templates := make(map[Kind]*Template, len(Kinds))
	for _, kind := range Kinds {
		text, err := builtinFS.ReadFile("templates/" + string(kind) + ".tmpl")
		if err != nil {
			return nil, fmt.Errorf("read built-in %s template: %w", kind, err)
		}
		template, err := Parse(kind, Version1, string(text))
		if err != nil {
			return nil, fmt.Errorf("parse built-in %s template: %w", kind, err)
		}
		templates[kind] = template
	}
	return templates, nil
}
