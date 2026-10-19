package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
	"github.com/patchcaster-app/patchcaster/internal/message"
)

func TestTheDescriptionNamesTheEnginesKinds(t *testing.T) {
	var described []message.Kind
	for _, kind := range v1gen.MessageKind("").AllValues() {
		described = append(described, message.Kind(kind))
	}

	assert.Equal(t, message.Kinds, described)
}

func TestAPreviewRendersEverySampleForMattermost(t *testing.T) {
	// arrange: anyone logged in may preview, bob is a Regular User.
	f := newFixture(t)
	draft := `{"kind":"root","version":1,"text":"{{mention \"here\"}} **{{.MergeRequest.Title}}**{{if not .MergeRequest.Open}} (closed){{end}}"}`

	// act
	rec := f.serve(t, request{method: http.MethodPost, path: "/api/v1/templates/preview", body: draft, session: f.sessionOf(t, bob)})

	// assert
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var preview v1gen.TemplatePreview
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
	require.NotEmpty(t, preview.Samples)
	assert.Equal(t, v1gen.TemplateSample{Description: "an open merge request", Mattermost: "@here **Add flux capacitor**"}, preview.Samples[0])
	assert.Contains(t, preview.Samples, v1gen.TemplateSample{Description: "a closed merge request", Mattermost: "@here **Add flux capacitor** (closed)"})
}

func TestARefusedTemplateNamesItsField(t *testing.T) {
	f := newFixture(t)
	session := f.sessionOf(t, bob)
	tests := []struct {
		name    string
		body    string
		path    string
		message string
	}{
		{"a function outside the set", `{"kind":"comment","version":1,"text":"ok\n{{printf \"%s\" .Event.Actor}}"}`, "text", "line 2: printf is not a template function"},
		{"a sample that fails", `{"kind":"root","version":1,"text":"{{if not .MergeRequest.Open}}{{.Nope}}{{end}}"}`, "text", "line 1: with a closed merge request: "},
		{"an unknown version", `{"kind":"root","version":2,"text":"x"}`, "version", "names no version of the data model"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := f.serve(t, request{method: http.MethodPost, path: "/api/v1/templates/preview", body: tc.body, session: session})

			require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
			p := readProblem(t, rec)
			require.Len(t, p.Errors, 1)
			assert.Equal(t, tc.path, p.Errors[0].Path)
			require.Len(t, p.Errors[0].Messages, 1)
			assert.Contains(t, p.Errors[0].Messages[0], tc.message)
		})
	}
}

func TestAPreviewNeedsALogin(t *testing.T) {
	f := newFixture(t)

	rec := f.serve(t, request{method: http.MethodPost, path: "/api/v1/templates/preview", body: `{"kind":"root","version":1,"text":"x"}`})

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
