package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/patchcaster-app/patchcaster/internal/store"
)

func TestGroupRefusal(t *testing.T) {
	limited := &store.LoginSettings{GroupsClaim: "groups", AllowedGroups: []string{"engineering", "ops"}}
	tests := []struct {
		name     string
		settings *store.LoginSettings
		claims   map[string]any
		refused  bool
	}{
		{"no limit admits everyone", &store.LoginSettings{}, map[string]any{}, false},
		{"a claim without allowed groups admits everyone", &store.LoginSettings{GroupsClaim: "groups"}, map[string]any{}, false},
		{"a member of an allowed group", limited, map[string]any{"groups": []any{"design", "ops"}}, false},
		{"a single group sent as a string", limited, map[string]any{"groups": "engineering"}, false},
		{"a member of other groups only", limited, map[string]any{"groups": []any{"design"}}, true},
		{"no groups claim at all", limited, map[string]any{}, true},
		{"a claim of the wrong shape", limited, map[string]any{"groups": 42}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			reason := groupRefusal(tc.settings, tc.claims)

			// assert
			assert.Equal(t, tc.refused, reason != "", "reason: %q", reason)
		})
	}
}

func TestDisplayNamePrefersTheName(t *testing.T) {
	tests := []struct {
		name   string
		claims *oidc.IDTokenClaims
		want   string
	}{
		{"the name", claimsWith("Ada Lovelace", "ada", "ada@example.com"), "Ada Lovelace"},
		{"the preferred username without a name", claimsWith("", "ada", "ada@example.com"), "ada"},
		{"the email without either", claimsWith("", "", "ada@example.com"), "ada@example.com"},
		{"the subject as a last resort", claimsWith("", "", ""), "subject-1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, displayName(tc.claims))
		})
	}
}

func claimsWith(name, username, email string) *oidc.IDTokenClaims {
	claims := &oidc.IDTokenClaims{}
	claims.Subject = "subject-1"
	claims.Name = name
	claims.PreferredUsername = username
	claims.Email = email
	return claims
}
