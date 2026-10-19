package message

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

// Error is why a template was refused, with the line it concerns. Line is 0
// when the cause concerns the whole template.
type Error struct {
	Line  int
	Cause string
}

// Error implements error.
func (e *Error) Error() string {
	if e.Line == 0 {
		return e.Cause
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Cause)
}

// templateLocation matches the start of text/template's errors: the
// template's name, the line, and for an execution error the column.
var templateLocation = regexp.MustCompile(`^template: [^:]*:(\d+):(?:\d+:)? ?`)

// templateError turns an error of text/template into an Error with its
// line.
func templateError(err error) *Error {
	message := err.Error()
	location := templateLocation.FindStringSubmatch(message)
	if location == nil {
		return &Error{Cause: strings.TrimPrefix(message, "template: ")}
	}
	line, _ := strconv.Atoi(location[1])
	return &Error{Line: line, Cause: message[len(location[0]):]}
}

// lineOf reads the line from a location of the form "name:line:column".
func lineOf(location string) int {
	parts := strings.Split(location, ":")
	if len(parts) < 2 {
		return 0
	}
	line, _ := strconv.Atoi(parts[1])
	return line
}

// Template is a parsed template for one kind of message, written for one
// version of the data model.
type Template struct {
	kind     Kind
	version  Version
	template *template.Template
}

// Parse reads the template text for messages of kind, written for version.
// It refuses text that does not parse, and text that calls a function
// outside the documented set.
func Parse(kind Kind, version Version, text string) (*Template, error) {
	if version != Version1 {
		return nil, &Error{Cause: fmt.Sprintf("data model version %d does not exist", version)}
	}
	parsed, err := template.New(string(kind)).Funcs(functions).Parse(text)
	if err != nil {
		return nil, templateError(err)
	}
	if len(parsed.Templates()) > 1 {
		return nil, &Error{Cause: "define and block are not available"}
	}
	if err := checkFunctions(parsed.Tree); err != nil {
		return nil, err
	}
	return &Template{kind: kind, version: version, template: parsed}, nil
}

// Kind returns the kind of message the template writes.
func (t *Template) Kind() Kind {
	return t.kind
}

// Version returns the version of the data model the template reads.
func (t *Template) Version() Version {
	return t.version
}

// Render writes the message about facts as Markdown.
func (t *Template) Render(facts Facts) (string, error) {
	var markdown strings.Builder
	if err := t.template.Execute(&markdown, dataV1(t.kind, facts)); err != nil {
		return "", templateError(err)
	}
	return markdown.String(), nil
}
