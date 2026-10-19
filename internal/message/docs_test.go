package message

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The user guide documents the data model, the kinds of messages and the
// built-in templates. These tests keep it in step with the code.

// templatesGuide reads the user guide on templates.
func templatesGuide(t *testing.T) string {
	t.Helper()
	guide, err := os.ReadFile("../../docs/templates.md")
	require.NoError(t, err)
	return string(guide)
}

// fieldPaths lists the paths of every leaf field below a struct type.
func fieldPaths(prefix string, structType reflect.Type) []string {
	var paths []string
	for field := range structType.Fields() {
		path := prefix + "." + field.Name
		if field.Type.Kind() == reflect.Struct {
			paths = append(paths, fieldPaths(path, field.Type)...)
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func TestGuideDocumentsEveryField(t *testing.T) {
	guide := templatesGuide(t)

	for _, path := range fieldPaths("", reflect.TypeFor[Data]()) {
		assert.Contains(t, guide, "| `"+path+"` |", "the guide's data model table lacks %s", path)
	}
}

func TestGuideNamesEveryKind(t *testing.T) {
	guide := templatesGuide(t)

	for _, kind := range Kinds {
		assert.Contains(t, guide, "| `"+string(kind)+"` |", "the guide's table of messages lacks %s", kind)
	}
}

func TestGuideShowsTheBuiltinTemplates(t *testing.T) {
	guide := templatesGuide(t)

	for _, kind := range Kinds {
		text, err := builtinFS.ReadFile("templates/" + string(kind) + ".tmpl")
		require.NoError(t, err)
		template := strings.TrimSuffix(string(text), "\n")
		if kind == Root {
			assert.Contains(t, guide, "```\n"+template+"\n```", "the guide shows another root")
			continue
		}
		assert.Contains(t, guide, "| `"+string(kind)+"` | `"+template+"` |", "the guide shows another %s template", kind)
	}
}
