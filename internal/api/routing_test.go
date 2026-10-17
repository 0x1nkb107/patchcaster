package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/internal/api/v1gen"
)

func TestAScopeIsParsed(t *testing.T) {
	tests := []struct {
		name     string
		kind     v1gen.SourceScopeKind
		path     string
		wantPath string
		wantErr  string
	}{
		{"the whole instance", v1gen.SourceScopeKindInstance, "", "", ""},
		{"a group", v1gen.SourceScopeKindGroup, "payments", "payments", ""},
		{"a subgroup with slashes around it", v1gen.SourceScopeKindGroup, "/payments/core/", "payments/core", ""},
		{"a repository", v1gen.SourceScopeKindRepository, "payments/api", "payments/api", ""},
		{"the instance with a path", v1gen.SourceScopeKindInstance, "payments", "", "must be empty for the whole instance"},
		{"a group without a path", v1gen.SourceScopeKindGroup, "", "", "names the group, such as payments"},
		{"a repository without its group", v1gen.SourceScopeKindRepository, "api", "", "names the repository with its group, such as payments/api"},
		{"an empty part", v1gen.SourceScopeKindGroup, "payments//core", "", "has an empty part"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			scope := v1gen.SourceScope{Kind: tc.kind}
			if tc.path != "" {
				scope.Path = v1gen.NewOptString(tc.path)
			}

			// act
			kind, path, err := parseScope(scope)

			// assert
			if tc.wantErr != "" {
				var problem *v1gen.ProblemStatusCode
				require.ErrorAs(t, err, &problem)
				require.Len(t, problem.Response.Errors, 1)
				assert.Equal(t, "scope.path", problem.Response.Errors[0].Path)
				assert.Equal(t, []string{tc.wantErr}, problem.Response.Errors[0].Messages)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, string(tc.kind), kind)
			assert.Equal(t, tc.wantPath, path)
		})
	}
}

func TestMutedRepliesAreKeptOnceInOrder(t *testing.T) {
	// act
	muted := mutedReplies([]v1gen.ReplyKind{v1gen.ReplyKindPush, v1gen.ReplyKindComment, v1gen.ReplyKindPush})
	none := mutedReplies(nil)

	// assert
	assert.Equal(t, []string{"comment", "push"}, muted)
	assert.Equal(t, []string{}, none, "no muted replies encode as an empty list")
}
